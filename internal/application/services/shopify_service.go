package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/AtRiskMedia/tractstack-go/internal/domain/entities/content"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/observability/logging"
	"github.com/AtRiskMedia/tractstack-go/internal/infrastructure/tenant"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/sync/singleflight"
)

// ShopifyService handles communication with Shopify APIs and webhook verification.
type ShopifyService struct {
	logger          *logging.ChanneledLogger
	tenantManager   *tenant.Manager
	resourceService *ResourceService
	requestGroup    singleflight.Group
}

// NewShopifyService creates a new Shopify service instance.
func NewShopifyService(logger *logging.ChanneledLogger, tenantManager *tenant.Manager, resourceService *ResourceService) *ShopifyService {
	return &ShopifyService{
		logger:          logger,
		tenantManager:   tenantManager,
		resourceService: resourceService,
	}
}

// VerifySignature validates the Shopify webhook HMAC signature.
func (s *ShopifyService) VerifySignature(tenantCtx *tenant.Context, body []byte, signature string) bool {
	secret := tenantCtx.Config.ShopifyAPISecret
	if secret == "" {
		s.logger.System().Warn("Shopify API Secret not configured for tenant", "tenantId", tenantCtx.TenantID)
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSignature := base64.StdEncoding.EncodeToString(expectedMAC)

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// ParseWebhook converts a raw Shopify webhook payload into a ResourceNode.
// It is resilient to minimal payloads and handles both numeric IDs and GID strings.
func (s *ShopifyService) ParseWebhook(body []byte) (*content.ResourceNode, error) {
	var rawData map[string]any
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook body: %w", err)
	}

	var idStr string
	if v, ok := rawData["id"].(float64); ok {
		idStr = fmt.Sprintf("%.0f", v)
	} else if v, ok := rawData["id"].(string); ok {
		idStr = v
	}

	if idStr == "" {
		return nil, fmt.Errorf("missing 'id' in webhook payload")
	}

	// Respect the user's observation: if it's already a GID, use it; otherwise, prefix it.
	gid := idStr
	if !strings.HasPrefix(idStr, "gid://") {
		gid = fmt.Sprintf("gid://shopify/Product/%s", idStr)
	}

	title, _ := rawData["title"].(string)
	handle, _ := rawData["handle"].(string)
	description, _ := rawData["body_html"].(string)

	oneLiner := ""
	if description != "" {
		oneLiner = bluemonday.StrictPolicy().Sanitize(description)
		if len(oneLiner) > 255 {
			oneLiner = oneLiner[:252] + "..."
		}
	}

	optionsPayload := make(map[string]any)
	optionsPayload["gid"] = gid
	jsonData, _ := json.Marshal(rawData)
	optionsPayload["shopifyData"] = string(jsonData)

	slug := ""
	if handle != "" {
		slug = fmt.Sprintf("product-%s", handle)
	}

	return &content.ResourceNode{
		Title:          title,
		Slug:           slug,
		OneLiner:       oneLiner,
		NodeType:       "Resource",
		OptionsPayload: optionsPayload,
	}, nil
}

// FetchProducts queries the Shopify Storefront API via GraphQL to get all products.
// It checks the backend cache first and uses singleflight to prevent thundering herds.
func (s *ShopifyService) FetchProducts(tenantCtx *tenant.Context) ([]byte, error) {
	// 1. Check Backend Cache (Fast Path)
	if cached, found := tenantCtx.CacheManager.GetShopifyCatalog(tenantCtx.TenantID); found {
		return cached, nil
	}

	// 2. Singleflight: Coalesce concurrent requests for the same tenant
	// We use the TenantID as the unique key so locking is isolated per tenant.
	key := fmt.Sprintf("shopify_fetch_%s", tenantCtx.TenantID)

	v, err, _ := s.requestGroup.Do(key, func() (interface{}, error) {
		// --- Start of Original Fetch Logic ---

		token := tenantCtx.Config.ShopifyStorefrontToken
		// Assuming ShopifyStoreDomain is available in config
		domain := tenantCtx.Config.ShopifyStoreDomain

		if token == "" || domain == "" {
			return nil, fmt.Errorf("shopify credentials (token/domain) missing for tenant %s", tenantCtx.TenantID)
		}

		cleanDomain := strings.TrimSuffix(domain, "/")
		// Ensure protocol
		if !strings.HasPrefix(cleanDomain, "http") {
			cleanDomain = "https://" + cleanDomain
		}
		url := fmt.Sprintf("%s/api/2024-01/graphql.json", cleanDomain)

		// GraphQL query to fetch all products (paginated)
		queryTemplate := `
        query ($cursor: String) {
          products(first: 250, after: $cursor, query: "product_type:'active'") {
            pageInfo {
              hasNextPage
              endCursor
            }
            edges {
              node {
                id
                title
                handle
                description
                options {
                  name
                  values
                }
                images(first: 20) {
                  edges {
                    node {
                      url
                      altText
                    }
                  }
                }
                variants(first: 250) {
                  edges {
                    node {
                      id
                      title
                      price {
                        amount
                        currencyCode
                      }
                      compareAtPrice {
                        amount
                        currencyCode
                      }
                      sku
                      availableForSale
                      requiresShipping
                      selectedOptions {
                        name
                        value
                      }
                    }
                  }
                }
              }
            }
          }
        }
    `

		var allEdges []any
		var cursor *string
		hasNextPage := true

		client := &http.Client{}

		for hasNextPage {
			reqBody := map[string]any{
				"query": queryTemplate,
				"variables": map[string]any{
					"cursor": cursor,
				},
			}
			jsonBody, _ := json.Marshal(reqBody)

			req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
			if err != nil {
				return nil, fmt.Errorf("failed to create shopify request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Shopify-Storefront-Access-Token", token)

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("shopify request failed: %w", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					s.logger.System().Warn("Failed to close Shopify response body", "error", err)
				}
			}()

			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("shopify api error: %s %s", resp.Status, string(b))
			}

			var result struct {
				Data struct {
					Products struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Edges []any `json:"edges"`
					} `json:"products"`
				} `json:"data"`
				Errors []any `json:"errors"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, fmt.Errorf("failed to decode shopify response: %w", err)
			}

			if len(result.Errors) > 0 {
				return nil, fmt.Errorf("graphql errors: %v", result.Errors)
			}

			allEdges = append(allEdges, result.Data.Products.Edges...)

			hasNextPage = result.Data.Products.PageInfo.HasNextPage
			if hasNextPage {
				c := result.Data.Products.PageInfo.EndCursor
				cursor = &c
			}
		}

		// Transform the edges into a flat list of products to simplify frontend consumption
		finalProducts := make([]map[string]any, 0, len(allEdges))
		for _, e := range allEdges {
			edge, ok := e.(map[string]any)
			if !ok {
				continue
			}
			node, ok := edge["node"].(map[string]any)
			if !ok {
				continue
			}

			// Flatten Images (edges -> nodes)
			if imgs, ok := node["images"].(map[string]any); ok {
				if iEdges, ok := imgs["edges"].([]any); ok {
					flatImages := make([]map[string]any, 0)
					for _, ie := range iEdges {
						if iEdge, ok := ie.(map[string]any); ok {
							if iNode, ok := iEdge["node"].(map[string]any); ok {
								flatImages = append(flatImages, iNode)
							}
						}
					}
					node["images"] = flatImages
				}
			}

			// Flatten Variants (edges -> nodes)
			if vars, ok := node["variants"].(map[string]any); ok {
				if vEdges, ok := vars["edges"].([]any); ok {
					flatVariants := make([]map[string]any, 0)
					for _, ve := range vEdges {
						if vEdge, ok := ve.(map[string]any); ok {
							if vNode, ok := vEdge["node"].(map[string]any); ok {
								flatVariants = append(flatVariants, vNode)
							}
						}
					}
					node["variants"] = flatVariants
				}
			}

			finalProducts = append(finalProducts, node)
		}

		response := map[string]any{
			"products": finalProducts,
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal shopify products: %w", err)
		}

		// Update Backend Cache (within the singleflight group)
		tenantCtx.CacheManager.SetShopifyCatalog(tenantCtx.TenantID, jsonData)

		return jsonData, nil
	})

	if err != nil {
		return nil, err
	}

	// Cast the interface{} return value back to []byte
	return v.([]byte), nil
}

// ReconcileAll performs a mass synchronization of all Shopify products for a tenant.
// It updates existing resources, creates new ones, and prunes orphaned resources
// that no longer exist on Shopify.
func (s *ShopifyService) ReconcileAll(tenantCtx *tenant.Context) (int, int, int, error) {
	// 1. Fetch all products currently active on Shopify
	productsJSON, err := s.FetchProducts(tenantCtx)
	if err != nil {
		return 0, 0, 0, err
	}

	var resp struct {
		Products []map[string]any `json:"products"`
	}
	if err := json.Unmarshal(productsJSON, &resp); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse products for reconciliation: %w", err)
	}

	// 2. Map incoming GIDs for fast diffing
	incomingGIDs := make(map[string]bool)
	for _, p := range resp.Products {
		if id, ok := p["id"].(string); ok {
			incomingGIDs[id] = true
		}
	}

	// 3. Prune Orphans: Find local resources that are no longer on Shopify
	deletedCount := 0

	// Get all local resources in categories managed by Shopify
	localProducts, _ := s.resourceService.GetByCategory(tenantCtx, "product")
	localServices, _ := s.resourceService.GetByCategory(tenantCtx, "service")

	// Fix for gocritic: Pre-allocate and append to the same slice to satisfy linter
	allLocal := make([]*content.ResourceNode, 0, len(localProducts)+len(localServices))
	allLocal = append(allLocal, localProducts...)
	allLocal = append(allLocal, localServices...)

	for _, local := range allLocal {
		gid, ok := local.OptionsPayload["gid"].(string)
		if !ok || gid == "" {
			continue // Not a Shopify-linked resource
		}

		if !incomingGIDs[gid] {
			// This GID exists locally but is missing from the Shopify fetch
			op, err := s.resourceService.SyncShopifyDeletion(tenantCtx, gid)
			if err != nil {
				s.logger.System().Error("Failed to prune orphaned Shopify resource",
					"error", err, "gid", gid, "tenantId", tenantCtx.TenantID)
				continue
			}
			if op == "deleted" {
				deletedCount++
			}
		}
	}

	// 4. Upsert path: Create or update products found in the fetch
	totalProcessed := len(resp.Products)
	reconciledCount := 0
	pCleaner := bluemonday.StrictPolicy()

	for _, p := range resp.Products {
		id, _ := p["id"].(string)
		handle, _ := p["handle"].(string)
		title, _ := p["title"].(string)
		description, _ := p["description"].(string)

		oneLiner := pCleaner.Sanitize(description)
		if len(oneLiner) > 255 {
			oneLiner = oneLiner[:252] + "..."
		}

		optionsPayload := make(map[string]any)
		optionsPayload["gid"] = id
		jsonData, _ := json.Marshal(p)
		optionsPayload["shopifyData"] = string(jsonData)

		resource := &content.ResourceNode{
			Title:          title,
			Slug:           fmt.Sprintf("product-%s", handle),
			OneLiner:       oneLiner,
			NodeType:       "Resource",
			OptionsPayload: optionsPayload,
		}

		op, err := s.resourceService.UpsertShopifyResource(tenantCtx, resource)
		if err != nil {
			s.logger.System().Error("Failed to reconcile product",
				"error", err, "handle", handle, "tenantId", tenantCtx.TenantID)
			continue
		}

		if op != "none" {
			reconciledCount++
		}
	}

	return totalProcessed, reconciledCount, deletedCount, nil
}
