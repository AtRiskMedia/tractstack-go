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
)

// ShopifyService handles communication with Shopify APIs and webhook verification.
type ShopifyService struct {
	logger        *logging.ChanneledLogger
	tenantManager *tenant.Manager
}

// NewShopifyService creates a new Shopify service instance.
func NewShopifyService(logger *logging.ChanneledLogger, tenantManager *tenant.Manager) *ShopifyService {
	return &ShopifyService{
		logger:        logger,
		tenantManager: tenantManager,
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
func (s *ShopifyService) ParseWebhook(body []byte) (*content.ResourceNode, error) {
	// 1. Unmarshal into a generic map to capture the full payload for storage
	var rawData map[string]any
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook body: %w", err)
	}

	// 2. Extract Key Fields safely
	// Shopify IDs in webhooks are numbers (e.g., 865236123), but we need GID format for consistency with Storefront API
	var idStr string
	if v, ok := rawData["id"].(float64); ok {
		idStr = fmt.Sprintf("%.0f", v)
	} else if v, ok := rawData["id"].(string); ok {
		idStr = v
	}

	if idStr == "" {
		return nil, fmt.Errorf("missing 'id' in webhook payload")
	}

	// Construct GID (Assuming Product for now - this logic might need expansion for Collections/Pages)
	gid := fmt.Sprintf("gid://shopify/Product/%s", idStr)

	title, _ := rawData["title"].(string)
	handle, _ := rawData["handle"].(string)
	description, _ := rawData["body_html"].(string) // Often body_html in REST hooks

	// Generate a simple one-liner (stripped of HTML tags ideally, but rough truncation for now)
	oneLiner := description
	if len(oneLiner) > 255 {
		oneLiner = oneLiner[:252] + "..."
	}

	// 3. Construct OptionsPayload
	optionsPayload := make(map[string]any)
	optionsPayload["gid"] = gid

	// Store the full raw data as a JSON string to match the frontend's previous behaviour
	// This allows the frontend components to hydrate from this blob without schema changes.
	jsonData, _ := json.Marshal(rawData)
	optionsPayload["shopifyData"] = string(jsonData)

	// 4. Construct ResourceNode
	// We generate a deterministic slug from the handle.
	// Note: The Upsert logic will use the GID to find the existing resource,
	// so if the slug changes in Shopify, we will handle the redirect/update in the Service layer.
	slug := fmt.Sprintf("product-%s", handle)

	resource := &content.ResourceNode{
		Title:          title,
		Slug:           slug,
		OneLiner:       oneLiner,
		NodeType:       "Resource",
		OptionsPayload: optionsPayload,
		// CategorySlug is intentionally left nil here; UpsertService will apply defaults/inheritance
	}

	return resource, nil
}

// FetchProducts queries the Shopify Storefront API via GraphQL to get all products.
// This replaces the logic previously held in the frontend `getProducts.ts`.
func (s *ShopifyService) FetchProducts(tenantCtx *tenant.Context) ([]byte, error) {
	token := tenantCtx.Config.ShopifyStorefrontToken
	// Assuming ShopifyStoreDomain is available in config (e.g., "my-shop.myshopify.com")
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

	return json.Marshal(response)
}
