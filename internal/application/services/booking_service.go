// Package services provides business logic and orchestration for the application.
package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/booking"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/domain/user"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ErrBookingNotFound used for error return value
var ErrBookingNotFound = errors.New("booking not found for trace ID")

const remoteConfirmationEmailMaxWait = 60 * time.Second

// BookingService handles business logic and orchestration for reservations.
type BookingService struct {
	logger            *logging.ChanneledLogger
	resourceService   *ResourceService
	emailWorker       *EmailWorker
	googleCalendarSvc *GoogleCalendarService
	locks             sync.Map // Maps tenantID string to *sync.Mutex for WAL-mode queueing
}

// NewBookingService creates a new booking service instance.
func NewBookingService(
	logger *logging.ChanneledLogger,
	resourceService *ResourceService,
	emailWorker *EmailWorker,
	googleCalendarSvc *GoogleCalendarService,
) *BookingService {
	return &BookingService{
		logger:            logger,
		resourceService:   resourceService,
		emailWorker:       emailWorker,
		googleCalendarSvc: googleCalendarSvc,
	}
}

// getTenantLock retrieves or creates a mutex for the specific tenant to prevent SQLite WAL-mode concurrency crashes.
func (s *BookingService) getTenantLock(tenantID string) *sync.Mutex {
	lock, _ := s.locks.LoadOrStore(tenantID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (s *BookingService) googleCalendarEnabled(tenantCtx *tenant.Context) bool {
	return s.googleCalendarSvc != nil && s.googleCalendarSvc.IsConfigured(tenantCtx)
}

func (s *BookingService) sendRemoteBookingConfirmationOnce(tenantCtx *tenant.Context, traceID string) {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	if s.emailWorker == nil {
		return
	}

	repo := tenantCtx.BookingRepo()
	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil || b == nil {
		return
	}
	if b.Status != booking.StatusConfirmed {
		return
	}
	if b.AppointmentMode != booking.AppointmentModeRemote {
		return
	}
	if b.ConfirmationEmailSent {
		return
	}

	lead, err := tenantCtx.LeadRepo().FindByID(b.LeadID)
	if err != nil || lead == nil || strings.TrimSpace(lead.Email) == "" {
		return
	}

	data := buildBookingTemplateData(tenantCtx, b, lead.FirstName, b.ShopifyOrderID)
	enqueued := s.emailWorker.Enqueue(EmailJob{
		TenantID:     tenantCtx.TenantID,
		To:           []string{lead.Email},
		Category:     "shopify",
		TemplateName: "booking-remote-confirmed",
		Data:         data,
	})
	if enqueued {
		_ = repo.MarkConfirmationEmailSent(tenantCtx.TenantID, traceID)
	}
}

// GetAvailability returns overlapping bookings for a given time window to support availability math.
func (s *BookingService) GetAvailability(tenantCtx *tenant.Context, start, end time.Time) ([]*booking.Booking, error) {
	repo := tenantCtx.BookingRepo()

	existingBookings, err := repo.FindOverlapping(tenantCtx.TenantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch overlapping bookings: %w", err)
	}
	if s.googleCalendarSvc != nil {
		busyRanges, googleErr := s.googleCalendarSvc.GetBusyRanges(context.Background(), tenantCtx, start, end)
		if googleErr != nil {
			s.logger.System().Warn("Failed to load Google busy ranges", "tenantId", tenantCtx.TenantID, "error", googleErr)
		} else {
			for idx, busy := range busyRanges {
				existingBookings = append(existingBookings, &booking.Booking{
					ID:               fmt.Sprintf("google_busy_%d", idx),
					ResourceIDs:      []string{},
					StartTime:        busy.Start.UTC(),
					EndTime:          busy.End.UTC(),
					Status:           booking.StatusConfirmed,
					AppointmentMode:  booking.AppointmentModeInPerson,
					GoogleSyncStatus: booking.GoogleSyncSynced,
					CreatedAt:        time.Now().UTC(),
				})
			}
		}
	}

	return existingBookings, nil
}

// HoldSlot attempts to lock a time slot for a user, using a tenant-level mutex before writing to the DB.
// It enforces UnavailableHours checks and saves the initial hold as pending.
func (s *BookingService) HoldSlot(ctx context.Context, tenantCtx *tenant.Context, traceID string, resourceIDs []string, leadID string, start, end time.Time, appointmentModeRaw string) error {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	// Check if client disconnected/timed out while waiting for the WAL queue
	if ctx.Err() != nil {
		return fmt.Errorf("client aborted request before lock acquisition: %w", ctx.Err())
	}

	// 0. Reject if in past
	if start.Before(time.Now()) {
		return fmt.Errorf("requested start time is in the past")
	}

	// Strict Backend Duration Validation ---
	// Fetch the requested resources to calculate the true required duration
	resources, err := s.resourceService.GetByIDs(tenantCtx, resourceIDs)
	if err != nil {
		return fmt.Errorf("failed to validate resource durations: %w", err)
	}

	var rawMinutes float64 = 0
	for _, res := range resources {
		if val, ok := res.OptionsPayload["bookingLengthMinutes"]; ok {
			// JSON unmarshaler converts numbers to float64 for any/interface{} types
			if minutes, ok := val.(float64); ok {
				rawMinutes += minutes
			}
		}
	}

	// Snap to 15-minute intervals matching the frontend's scheduling grid
	interval := 15.0
	snappedMinutes := math.Ceil(rawMinutes/interval) * interval

	maxLength := float64(tenantCtx.Config.BrandConfig.Scheduling.MaxLengthMinutes)
	if snappedMinutes > maxLength {
		snappedMinutes = maxLength
	}

	// Enforce a minimum 15m duration if a booking is required but missing duration config
	if snappedMinutes == 0 && len(resources) > 0 {
		snappedMinutes = 15.0
	}

	// Override the user-provided end time with the securely calculated end time
	secureEnd := start.Add(time.Duration(snappedMinutes) * time.Minute)

	// Final sanity check against global max length limit
	if secureEnd.Sub(start).Minutes() > maxLength {
		return fmt.Errorf("requested duration exceeds maximum allowed length")
	}
	// -----------------------------------------------

	// 1. Check Unavailable Hours (Strict Backend Validation)
	for _, block := range tenantCtx.Config.BrandConfig.Scheduling.UnavailableHours {
		blockStart, err1 := time.Parse(time.RFC3339, block.Start)
		blockEnd, err2 := time.Parse(time.RFC3339, block.End)
		if err1 == nil && err2 == nil {
			if start.Before(blockEnd) && secureEnd.After(blockStart) {
				return fmt.Errorf("time slot overlaps with unavailable hours")
			}
		}
	}

	repo := tenantCtx.BookingRepo()

	// 2. Check for overlapping database bookings EXACTLY within the locked context
	overlapping, err := repo.FindOverlapping(tenantCtx.TenantID, start, secureEnd)
	if err != nil {
		return fmt.Errorf("failed to check availability: %w", err)
	}

	if len(overlapping) > 0 {
		return fmt.Errorf("time slot is no longer available")
	}

	serviceResources, err := s.resolveBookingServiceResources(tenantCtx, resources)
	if err != nil {
		return fmt.Errorf("failed to resolve booking service resources: %w", err)
	}

	appointmentMode := booking.AppointmentMode(strings.ToUpper(strings.TrimSpace(appointmentModeRaw)))
	if appointmentMode != booking.AppointmentModeRemote {
		appointmentMode = booking.AppointmentModeInPerson
	}
	appointmentMode, err = s.resolveAppointmentMode(tenantCtx, appointmentMode, serviceResources)
	if err != nil {
		return err
	}

	// 3. Create the Booking (Defaulting to Pending)
	newBooking := &booking.Booking{
		ID:                    traceID,
		ResourceIDs:           resourceIDs,
		LeadID:                leadID,
		StartTime:             start,
		EndTime:               secureEnd,
		Status:                booking.StatusPending,
		AppointmentMode:       appointmentMode,
		GoogleSyncStatus:      booking.GoogleSyncNotSynced,
		ConfirmationEmailSent: false,
		LinkAddedEmailSent:    false,
		CreatedAt:             time.Now().UTC(),
	}

	if err := repo.Store(tenantCtx.TenantID, newBooking); err != nil {
		return fmt.Errorf("failed to store booking hold: %w", err)
	}

	s.logger.System().Info("Booking slot held successfully",
		"traceId", traceID,
		"tenantId", tenantCtx.TenantID,
		"durationMinutes", snappedMinutes)

	return nil
}

func (s *BookingService) resolveBookingServiceResources(tenantCtx *tenant.Context, resources []*content.ResourceNode) ([]*content.ResourceNode, error) {
	serviceMap := map[string]*content.ResourceNode{}

	for _, resource := range resources {
		if resource == nil {
			continue
		}

		if (resource.CategorySlug != nil && *resource.CategorySlug == "service") || hasBookingLength(resource.OptionsPayload) {
			serviceMap[resource.ID] = resource
		}

		if boundSlug, ok := resource.OptionsPayload["serviceBound"].(string); ok && strings.TrimSpace(boundSlug) != "" {
			boundService, err := s.resourceService.GetBySlug(tenantCtx, boundSlug)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve serviceBound resource %q: %w", boundSlug, err)
			}
			if boundService != nil {
				serviceMap[boundService.ID] = boundService
			}
		}
	}

	out := make([]*content.ResourceNode, 0, len(serviceMap))
	for _, service := range serviceMap {
		out = append(out, service)
	}
	return out, nil
}

func (s *BookingService) resolveAppointmentMode(
	tenantCtx *tenant.Context,
	requestedMode booking.AppointmentMode,
	serviceResources []*content.ResourceNode,
) (booking.AppointmentMode, error) {
	scheduling := tenantCtx.Config.BrandConfig.Scheduling
	if scheduling.RemoteOnly {
		return booking.AppointmentModeRemote, nil
	}

	if requestedMode == booking.AppointmentModeRemote && !scheduling.AllowRemote {
		return "", fmt.Errorf("remote booking is disabled for this tenant")
	}

	finalMode := requestedMode
	if finalMode != booking.AppointmentModeRemote {
		finalMode = booking.AppointmentModeInPerson
	}

	for _, resource := range serviceResources {
		allowRemote, remoteOnly := serviceRemoteFlags(resource.OptionsPayload)
		if remoteOnly {
			allowRemote = true
			finalMode = booking.AppointmentModeRemote
		}

		if finalMode == booking.AppointmentModeRemote && !allowRemote {
			return "", fmt.Errorf("service %s does not allow remote booking", resource.ID)
		}
	}

	return finalMode, nil
}

func hasBookingLength(payload map[string]interface{}) bool {
	_, ok := payload["bookingLengthMinutes"]
	return ok
}

func serviceRemoteFlags(payload map[string]interface{}) (allowRemote bool, remoteOnly bool) {
	allowRemote = parseBoolPayload(payload["allowRemote"])
	remoteOnly = parseBoolPayload(payload["remoteOnly"])
	if remoteOnly {
		allowRemote = true
	}
	return
}

func parseBoolPayload(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

// ConfirmBooking finalizes a pending booking, transitioning it to CONFIRMED.
func (s *BookingService) ConfirmBooking(tenantCtx *tenant.Context, traceID string, shopifyOrderID *string) error {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	repo := tenantCtx.BookingRepo()

	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil {
		return fmt.Errorf("failed to retrieve booking for confirmation: %w", err)
	}
	if b == nil {
		return ErrBookingNotFound
	}
	// Verify free cart bypass is legitimate
	if shopifyOrderID == nil {
		resources, err := s.resourceService.GetByIDs(tenantCtx, b.ResourceIDs)
		if err != nil {
			return fmt.Errorf("failed to retrieve resources for validation: %w", err)
		}
		for _, res := range resources {
			if _, hasGID := res.OptionsPayload["gid"]; hasGID {
				return fmt.Errorf("unauthorized: cannot natively confirm paid resource %s", res.ID)
			}
		}
	}

	if err := repo.UpdateStatus(tenantCtx.TenantID, traceID, booking.StatusConfirmed, shopifyOrderID); err != nil {
		return fmt.Errorf("failed to confirm booking: %w", err)
	}

	s.logger.System().Info("Booking confirmed", "traceId", traceID, "tenantId", tenantCtx.TenantID)

	lead, _ := tenantCtx.LeadRepo().FindByID(b.LeadID)

	if s.googleCalendarEnabled(tenantCtx) {
		go s.syncConfirmedBookingToGoogle(tenantCtx, traceID)
	}

	deferRemoteEmail := b.AppointmentMode == booking.AppointmentModeRemote && s.googleCalendarEnabled(tenantCtx)

	if s.emailWorker != nil && lead != nil && lead.Email != "" {
		if deferRemoteEmail {
			time.AfterFunc(remoteConfirmationEmailMaxWait, func() {
				s.sendRemoteBookingConfirmationOnce(tenantCtx, traceID)
			})
		} else {
			data := buildBookingTemplateData(tenantCtx, b, lead.FirstName, shopifyOrderID)
			templateName := "booking-confirmed"
			if b.AppointmentMode == booking.AppointmentModeRemote {
				templateName = "booking-remote-confirmed"
			}
			enqueued := s.emailWorker.Enqueue(EmailJob{
				TenantID:     tenantCtx.TenantID,
				To:           []string{lead.Email},
				Category:     "shopify",
				TemplateName: templateName,
				Data:         data,
			})
			if enqueued {
				_ = repo.MarkConfirmationEmailSent(tenantCtx.TenantID, traceID)
			}
		}
	}

	return nil
}

// ReleaseHold drops a pending booking proactively
func (s *BookingService) ReleaseHold(tenantCtx *tenant.Context, traceID string) error {
	repo := tenantCtx.BookingRepo()
	return repo.DeletePendingByTraceID(tenantCtx.TenantID, traceID)
}

// ListBookings retrieves a paginated list of bookings for the administrative dashboard.
func (s *BookingService) ListBookings(tenantCtx *tenant.Context, limit, offset int, status string) ([]*booking.Booking, int, error) {
	repo := tenantCtx.BookingRepo()

	bookings, count, err := repo.FindAllPaginated(tenantCtx.TenantID, limit, offset, status)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list bookings: %w", err)
	}

	return bookings, count, nil
}

// GetMetrics calculates aggregated booking volume and conversion statistics.
func (s *BookingService) GetMetrics(tenantCtx *tenant.Context) (*booking.BookingMetrics, error) {
	repo := tenantCtx.BookingRepo()
	now := time.Now().UTC()

	metrics, err := repo.GetMetrics(tenantCtx.TenantID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate booking metrics: %w", err)
	}

	return metrics, nil
}

// CancelBooking manually transitions a booking to the CANCELLED state.
// It acquires a tenant-level lock to prevent race conditions with incoming webhooks.
func (s *BookingService) CancelBooking(tenantCtx *tenant.Context, traceID string) error {
	mu := s.getTenantLock(tenantCtx.TenantID)
	mu.Lock()
	defer mu.Unlock()

	repo := tenantCtx.BookingRepo()

	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil {
		return fmt.Errorf("failed to retrieve booking for cancellation: %w", err)
	}
	if b == nil {
		return fmt.Errorf("booking not found for trace ID: %s", traceID)
	}

	if err := repo.UpdateStatus(tenantCtx.TenantID, traceID, booking.StatusCancelled, nil); err != nil {
		return fmt.Errorf("failed to cancel booking: %w", err)
	}

	s.logger.System().Info("Booking manually cancelled", "traceId", traceID, "tenantId", tenantCtx.TenantID)

	if s.emailWorker != nil {
		lead, _ := tenantCtx.LeadRepo().FindByID(b.LeadID)
		if lead != nil {
			orderID := traceID
			_ = s.emailWorker.Enqueue(EmailJob{
				TenantID:     tenantCtx.TenantID,
				To:           []string{lead.Email},
				Category:     "shopify",
				TemplateName: "booking-cancelled",
				Data: map[string]any{
					"LeadName":       lead.FirstName,
					"ShopifyOrderID": orderID,
				},
			})
		}
	}

	if s.googleCalendarEnabled(tenantCtx) {
		go s.syncCancelledBookingToGoogle(tenantCtx, traceID)
	}

	return nil
}

func (s *BookingService) syncConfirmedBookingToGoogle(tenantCtx *tenant.Context, traceID string) {
	repo := tenantCtx.BookingRepo()
	if err := repo.UpdateGoogleSyncPending(tenantCtx.TenantID, traceID); err != nil {
		s.logger.System().Warn("Failed to mark google sync pending", "tenantId", tenantCtx.TenantID, "traceId", traceID, "error", err)
	}

	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil || b == nil {
		s.logger.System().Warn("Failed to load booking for google create", "tenantId", tenantCtx.TenantID, "traceId", traceID, "error", err)
		return
	}
	if b.Status == booking.StatusCancelled {
		return
	}

	lead, _ := tenantCtx.LeadRepo().FindByID(b.LeadID)
	resources, resErr := s.resourceService.GetByIDs(tenantCtx, b.ResourceIDs)
	if resErr != nil {
		s.logger.System().Warn("Failed to load resources for google calendar copy", "tenantId", tenantCtx.TenantID, "traceId", traceID, "error", resErr)
		resources = nil
	}
	summary, description := buildCalendarEventCopy(tenantCtx, b, lead, resources)

	eventID, meetURL, err := s.googleCalendarSvc.CreateBookingEvent(context.Background(), tenantCtx, b, summary, description)
	if err != nil {
		_ = repo.UpdateGoogleSyncFailure(tenantCtx.TenantID, traceID, booking.GoogleSyncFailed, err.Error())
		s.logger.System().Warn("Google create event failed", "tenantId", tenantCtx.TenantID, "traceId", traceID, "error", err)
		if b.AppointmentMode == booking.AppointmentModeRemote {
			s.sendRemoteBookingConfirmationOnce(tenantCtx, traceID)
		}
		return
	}
	if err := repo.UpdateGoogleSyncSuccess(tenantCtx.TenantID, traceID, eventID, meetURL); err != nil {
		s.logger.System().Warn("Failed to persist google sync success", "tenantId", tenantCtx.TenantID, "traceId", traceID, "error", err)
	}

	if b.AppointmentMode == booking.AppointmentModeRemote {
		s.sendRemoteBookingConfirmationOnce(tenantCtx, traceID)
	}
}

func (s *BookingService) syncCancelledBookingToGoogle(tenantCtx *tenant.Context, traceID string) {
	repo := tenantCtx.BookingRepo()
	if err := repo.UpdateGoogleDeletePending(tenantCtx.TenantID, traceID); err != nil {
		s.logger.System().Warn("Failed to mark google delete pending", "tenantId", tenantCtx.TenantID, "traceId", traceID, "error", err)
	}

	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil || b == nil {
		return
	}

	eventID := b.GoogleEventID
	if eventID == nil || *eventID == "" {
		lookupEventID, lookupErr := s.googleCalendarSvc.FindEventIDByBookingID(context.Background(), tenantCtx, traceID)
		if lookupErr != nil {
			_ = repo.UpdateGoogleSyncFailure(tenantCtx.TenantID, traceID, booking.GoogleSyncFailed, lookupErr.Error())
			return
		}
		eventID = lookupEventID
	}
	if eventID == nil || *eventID == "" {
		_ = repo.UpdateGoogleSyncFailure(tenantCtx.TenantID, traceID, booking.GoogleSyncFailed, "google event id not found for cancellation")
		return
	}

	if err := s.googleCalendarSvc.DeleteEvent(context.Background(), tenantCtx, *eventID); err != nil {
		_ = repo.UpdateGoogleSyncFailure(tenantCtx.TenantID, traceID, booking.GoogleSyncFailed, err.Error())
		s.logger.System().Warn("Google delete event failed", "tenantId", tenantCtx.TenantID, "traceId", traceID, "eventId", *eventID, "error", err)
		return
	}
	if err := repo.UpdateGoogleDeleteSuccess(tenantCtx.TenantID, traceID); err != nil {
		s.logger.System().Warn("Failed to persist google delete success", "tenantId", tenantCtx.TenantID, "traceId", traceID, "error", err)
	}
}

// buildCalendarEventCopy produces Google Calendar summary and description for the business owner.
func buildCalendarEventCopy(
	tenantCtx *tenant.Context,
	b *booking.Booking,
	lead *user.Lead,
	resources []*content.ResourceNode,
) (summary string, description string) {
	displayName, contactEmail := leadCalendarDisplayNameAndEmail(lead, b)
	byID := resourceNodesByID(resources)

	n := len(b.ResourceIDs)
	var summaryBody string
	switch {
	case n == 0:
		summaryBody = fmt.Sprintf("Booking – %s", displayName)
	case n == 1:
		id := b.ResourceIDs[0]
		st := strings.TrimSpace(resourceTitleForCalendar(byID[id], id))
		if st == "" {
			st = "Service"
		}
		summaryBody = fmt.Sprintf("%s – %s", st, displayName)
	default:
		summaryBody = fmt.Sprintf("%d services – %s", n, displayName)
	}
	if b.AppointmentMode == booking.AppointmentModeRemote {
		summary = "Remote · " + summaryBody
	} else {
		summary = summaryBody
	}

	bookingTZ := tenantCtx.Config.BrandConfig.Scheduling.Timezone
	if bookingTZ == "" {
		bookingTZ = "UTC"
	}
	loc, err := time.LoadLocation(bookingTZ)
	if err != nil {
		loc = time.UTC
		bookingTZ = "UTC"
	}
	startDisplay := b.StartTime.In(loc).Format("Jan 2, 2006 3:04 PM")
	endDisplay := b.EndTime.In(loc).Format("Jan 2, 2006 3:04 PM")
	bookingFor := fmt.Sprintf("%s to %s (%s)", startDisplay, endDisplay, bookingTZ)

	var formatLine string
	if b.AppointmentMode == booking.AppointmentModeRemote {
		formatLine = "Format: Remote (Google Meet link is on the calendar event)"
	} else {
		formatLine = "Format: In person"
	}

	var svcBlock strings.Builder
	svcBlock.WriteString("Services:")
	for _, id := range b.ResourceIDs {
		title := strings.TrimSpace(resourceTitleForCalendar(byID[id], id))
		if title == "" {
			title = id
		}
		svcBlock.WriteString("\n- ")
		svcBlock.WriteString(title)
	}

	var contactLine string
	if strings.TrimSpace(contactEmail) != "" {
		contactLine = fmt.Sprintf("Contact: %s <%s>", displayName, strings.TrimSpace(contactEmail))
	} else {
		contactLine = fmt.Sprintf("Contact: %s", displayName)
	}

	blocks := []string{bookingFor, formatLine, svcBlock.String(), contactLine}
	if b.ShopifyOrderID != nil && strings.TrimSpace(*b.ShopifyOrderID) != "" {
		oid := strings.TrimSpace(*b.ShopifyOrderID)
		blocks = append(blocks, fmt.Sprintf("Order: %s", oid))
	}
	blocks = append(blocks, fmt.Sprintf("Booking ref: %s", b.ID))
	description = strings.Join(blocks, "\n")
	return summary, description
}

func leadCalendarDisplayNameAndEmail(lead *user.Lead, b *booking.Booking) (displayName string, email string) {
	if lead != nil {
		email = strings.TrimSpace(lead.Email)
		fn := strings.TrimSpace(lead.FirstName)
		if fn != "" {
			displayName = fn
		} else if email != "" {
			if at := strings.Index(email, "@"); at > 0 {
				displayName = strings.TrimSpace(email[:at])
			}
		}
	}
	if displayName == "" {
		displayName = b.LeadID
	}
	return displayName, email
}

func resourceNodesByID(nodes []*content.ResourceNode) map[string]*content.ResourceNode {
	m := make(map[string]*content.ResourceNode, len(nodes))
	for _, n := range nodes {
		if n != nil {
			m[n.ID] = n
		}
	}
	return m
}

func resourceTitleForCalendar(r *content.ResourceNode, idFallback string) string {
	if r != nil && strings.TrimSpace(r.Title) != "" {
		return r.Title
	}
	return idFallback
}

func buildBookingTemplateData(
	tenantCtx *tenant.Context,
	b *booking.Booking,
	leadName string,
	shopifyOrderID *string,
) map[string]any {
	bookingTZ := tenantCtx.Config.BrandConfig.Scheduling.Timezone
	if bookingTZ == "" {
		bookingTZ = "UTC"
	}
	loc, err := time.LoadLocation(bookingTZ)
	if err != nil {
		loc = time.UTC
		bookingTZ = "UTC"
	}
	startDisplay := b.StartTime.In(loc).Format("Jan 2, 2006 3:04 PM")
	endDisplay := b.EndTime.In(loc).Format("Jan 2, 2006 3:04 PM")
	bookingFor := fmt.Sprintf("%s to %s (%s)", startDisplay, endDisplay, bookingTZ)
	data := map[string]any{
		"LeadName":                leadName,
		"BookingID":               b.ID,
		"BookingTimezone":         bookingTZ,
		"BookingStartDisplay":     startDisplay,
		"BookingEndDisplay":       endDisplay,
		"BookingForDisplayString": bookingFor,
	}
	if shopifyOrderID != nil {
		data["ShopifyOrderID"] = *shopifyOrderID
	}
	if b.GoogleMeetURL != nil {
		data["GoogleMeetURL"] = *b.GoogleMeetURL
	}
	return data
}
