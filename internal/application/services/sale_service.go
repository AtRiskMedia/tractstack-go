// Package services provides business logic and orchestration for the application.
package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	saleEntity "github.com/AtRiskMedia/tractstack-go/internal/domain/entities/sale"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
)

// ShopifyPaidOrder captures the fields used from Shopify's orders/paid REST webhook.
type ShopifyPaidOrder struct {
	ID             int64                  `json:"id"`
	TotalPrice     string                 `json:"total_price"`
	NoteAttributes []ShopifyNoteAttribute `json:"note_attributes"`
	LineItems      []ShopifyPaidOrderLine `json:"line_items"`
}

// ShopifyNoteAttribute is a cart/order attribute projected into a paid order.
type ShopifyNoteAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ShopifyPaidOrderLine captures immutable checkout line data from Shopify.
type ShopifyPaidOrderLine struct {
	VariantID int64  `json:"variant_id"`
	Quantity  int    `json:"quantity"`
	Title     string `json:"title"`
	Price     string `json:"price"`
	PriceSet  struct {
		ShopMoney struct {
			CurrencyCode string `json:"currency_code"`
		} `json:"shop_money"`
	} `json:"price_set"`
}

// SaleService orchestrates Shopify paid-order receipt persistence and listing.
type SaleService struct {
	resourceService *ResourceService
	bookingService  *BookingService
	emailWorker     *EmailWorker
	logger          *logging.ChanneledLogger
}

// NewSaleService creates a SaleService instance.
func NewSaleService(resourceService *ResourceService, bookingService *BookingService, emailWorker *EmailWorker, logger *logging.ChanneledLogger) *SaleService {
	return &SaleService{
		resourceService: resourceService,
		bookingService:  bookingService,
		emailWorker:     emailWorker,
		logger:          logger,
	}
}

// ProcessOrdersPaid persists the paid order receipt, confirms associated bookings, and reports true orphan payments.
func (s *SaleService) ProcessOrdersPaid(tenantCtx *tenant.Context, order ShopifyPaidOrder) error {
	attrs := make(map[string]string, len(order.NoteAttributes))
	for _, attr := range order.NoteAttributes {
		attrs[attr.Name] = attr.Value
	}

	traceID := strings.TrimSpace(attrs["bookingId"])
	if traceID == "" {
		traceID = strings.TrimSpace(attrs["Trace ID"])
	}
	if traceID == "" {
		return fmt.Errorf("orders/paid missing bookingId trace attribute")
	}

	hasAppointmentIntent := attrs["Appointment Date"] != "" ||
		attrs["Appointment Time"] != "" ||
		attrs["Appointment Mode"] != ""

	repo := tenantCtx.BookingRepo()
	b, err := repo.FindByID(tenantCtx.TenantID, traceID)
	if err != nil {
		return fmt.Errorf("failed to retrieve booking for paid order: %w", err)
	}

	leadID := strings.TrimSpace(attrs["leadId"])
	if leadID == "" && b != nil {
		leadID = b.LeadID
	}

	products, err := s.buildSaleProductLines(tenantCtx, order.LineItems)
	if err != nil {
		return err
	}

	var bookingID *string
	if b != nil {
		bookingID = &traceID
	}

	orderID := strconv.FormatInt(order.ID, 10)
	sale := &saleEntity.Sale{
		ID:                traceID,
		LeadID:            leadID,
		BookingID:         bookingID,
		ShopifyOrderID:    orderID,
		TotalAmount:       order.TotalPrice,
		Status:            saleEntity.SaleStatusPaid,
		Products:          products,
		AppointmentIntent: hasAppointmentIntent,
	}

	if err := tenantCtx.SaleRepo().UpsertByShopifyOrderID(tenantCtx.TenantID, sale); err != nil {
		return fmt.Errorf("failed to upsert paid sale: %w", err)
	}

	if b != nil {
		if err := s.bookingService.ConfirmBooking(tenantCtx, traceID, &orderID); err != nil {
			return fmt.Errorf("failed to confirm booking from paid order: %w", err)
		}
		return nil
	}

	if hasAppointmentIntent {
		s.enqueueOrphanedPaymentEmail(tenantCtx, traceID, orderID)
	}

	return nil
}

// List returns the paginated sales list with computed dashboard tags.
func (s *SaleService) List(tenantCtx *tenant.Context, limit, offset int) ([]*saleEntity.SaleListItem, int, error) {
	items, totalCount, err := tenantCtx.SaleRepo().FindAllPaginatedWithBooking(tenantCtx.TenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sales: %w", err)
	}

	for _, item := range items {
		item.Tags = buildSaleTags(item)
	}

	return items, totalCount, nil
}

// GetMetrics calculates paid Shopify receipt aggregates for the dashboard.
func (s *SaleService) GetMetrics(tenantCtx *tenant.Context) (*saleEntity.SaleMetrics, error) {
	repo := tenantCtx.SaleRepo()
	now := time.Now().UTC()

	metrics, err := repo.GetMetrics(tenantCtx.TenantID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate sale metrics: %w", err)
	}

	return metrics, nil
}

func (s *SaleService) buildSaleProductLines(tenantCtx *tenant.Context, lines []ShopifyPaidOrderLine) ([]saleEntity.SaleProductLine, error) {
	products := make([]saleEntity.SaleProductLine, 0, len(lines))
	for _, line := range lines {
		productLine, err := s.variantToProductResource(tenantCtx, line)
		if err != nil {
			return nil, err
		}
		products = append(products, productLine)
	}
	return products, nil
}

func (s *SaleService) variantToProductResource(tenantCtx *tenant.Context, line ShopifyPaidOrderLine) (saleEntity.SaleProductLine, error) {
	paidVariantGID := fmt.Sprintf("gid://shopify/ProductVariant/%d", line.VariantID)
	numericID := strconv.FormatInt(line.VariantID, 10)

	products, err := s.resourceService.GetByCategory(tenantCtx, "product")
	if err != nil {
		return saleEntity.SaleProductLine{}, err
	}

	for _, product := range products {
		if product == nil {
			continue
		}
		parsed, err := parseShopifyData(product)
		if err != nil {
			return saleEntity.SaleProductLine{}, err
		}
		if !shopifyDataHasVariant(parsed, paidVariantGID, numericID) {
			continue
		}

		gid, _ := product.OptionsPayload["gid"].(string)
		currencyCode := strings.TrimSpace(line.PriceSet.ShopMoney.CurrencyCode)
		if currencyCode == "" {
			currencyCode = "USD"
		}

		return saleEntity.SaleProductLine{
			ResourceID:    product.ID,
			GID:           gid,
			VariantID:     paidVariantGID,
			Quantity:      line.Quantity,
			Title:         line.Title,
			Price:         line.Price,
			CurrencyCode:  currencyCode,
			IsLocalPickup: isPickupVariant(parsed, paidVariantGID),
		}, nil
	}

	return saleEntity.SaleProductLine{}, fmt.Errorf("failed to map Shopify variant %s to a product resource", paidVariantGID)
}

func parseShopifyData(product *content.ResourceNode) (map[string]any, error) {
	raw, ok := product.OptionsPayload["shopifyData"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("product resource %s missing shopifyData", product.ID)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse shopifyData for resource %s: %w", product.ID, err)
	}
	return parsed, nil
}

func shopifyDataHasVariant(parsed map[string]any, variantGID, numericID string) bool {
	for _, variant := range toMapSlice(parsed["variants"]) {
		id, _ := variant["id"].(string)
		if id == variantGID || strings.HasSuffix(id, numericID) {
			return true
		}
	}
	return false
}

func isPickupVariant(parsed map[string]any, paidVariantGID string) bool {
	for _, variant := range toMapSlice(parsed["variants"]) {
		id, _ := variant["id"].(string)
		if id != paidVariantGID {
			continue
		}
		for _, option := range toMapSlice(variant["selectedOptions"]) {
			name, _ := option["name"].(string)
			value, _ := option["value"].(string)
			if name == "Mode" && value == "Pickup" {
				return true
			}
		}
	}
	return false
}

func buildSaleTags(item *saleEntity.SaleListItem) []string {
	tags := make([]string, 0, 3)
	for _, product := range item.Products {
		if product.IsLocalPickup {
			tags = append(tags, "local-pickup")
			break
		}
	}
	if item.AppointmentIntent && item.BookingID == nil {
		tags = append(tags, "orphan")
	}
	if item.Booking != nil {
		mode := strings.ToLower(string(item.Booking.AppointmentMode))
		mode = strings.ReplaceAll(mode, "_", "-")
		if mode != "" {
			tags = append(tags, mode)
		}
	}
	return tags
}

func (s *SaleService) enqueueOrphanedPaymentEmail(tenantCtx *tenant.Context, traceID, orderID string) {
	s.logger.System().Error("ORPHANED PAYMENT: User paid for an expired booking hold", "traceId", traceID, "shopifyOrderId", orderID)
	if s.emailWorker == nil || tenantCtx.Config.BrandConfig == nil || tenantCtx.Config.BrandConfig.AdminEmail == "" {
		return
	}
	_ = s.emailWorker.Enqueue(EmailJob{
		TenantID:     tenantCtx.TenantID,
		To:           []string{tenantCtx.Config.BrandConfig.AdminEmail},
		Category:     "shopify",
		TemplateName: "orphaned-payment",
		Data: map[string]any{
			"TraceID":        traceID,
			"ShopifyOrderID": orderID,
		},
	})
}
