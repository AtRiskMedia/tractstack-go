package services

import (
	"sort"
	"strings"
)

// Core IDs in the order templates/custom/minimal/codehooks.ts lists them
// (shopify ids are registry/platform, not core — they come from hooks[]).
var managedManifestCoreIDs = []string{
	"featured-article",
	"list-content",
	"search-widget",
	"bunny-video",
	"epinet",
}

const managedManifestComment = `// Static, hand-maintained per-install manifest of available codehook ids.
// The backend never sees this; it is a frontend build capability list only.
// To add/remove a hook, edit this array AND add/remove the matching blade at
// src/pages/codehooks/{id}.astro. Not regenerated automatically.
`

// GenerateHookScaffold returns the Phase 3 hook trunk bytes (§5 Generators).
// kind is "astro" or "astro+jsx"; it changes only the kind comment line.
func GenerateHookScaffold(id, kind string) string {
	kindLine := "// Import Astro components from '@/custom/…' and render them below."
	if kind == "astro+jsx" {
		kindLine = `// Import a React island from '@/custom/…' and render it with client:load or client:only="react".`
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("export const partial = true;\n")
	b.WriteString("// CodeHook \"")
	b.WriteString(id)
	b.WriteString("\". Edit freely; this file is yours.\n")
	b.WriteString("// Props arrive as query params: ?paneId=…&tenantId=…&options=<JSON string>\n")
	b.WriteString(kindLine)
	b.WriteString("\n")
	b.WriteString("const options = Astro.url.searchParams.get('options');\n")
	b.WriteString("---\n\n")
	b.WriteString("<div data-codehook=\"")
	b.WriteString(id)
	b.WriteString("\">\n")
	b.WriteString("  <p>CodeHook <code>")
	b.WriteString(id)
	b.WriteString("</code> — replace this with your markup.</p>\n")
	b.WriteString("</div>\n")
	return b.String()
}

// GenerateManagedManifest regenerates src/custom/codehooks.ts (§5).
// Core ids first (minimal template order), then registry hook ids sorted.
func GenerateManagedManifest(hookIDs []string) string {
	seen := make(map[string]struct{}, len(hookIDs))
	var extra []string
	for _, id := range hookIDs {
		if id == "" || IsCoreBladeID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		extra = append(extra, id)
	}
	sort.Strings(extra)

	var b strings.Builder
	b.WriteString(managedManifestComment)
	b.WriteString("export const availableCodeHookIds: string[] = [\n")
	for _, id := range managedManifestCoreIDs {
		b.WriteString("  '")
		b.WriteString(id)
		b.WriteString("',\n")
	}
	for _, id := range extra {
		b.WriteString("  '")
		b.WriteString(id)
		b.WriteString("',\n")
	}
	b.WriteString("];\n")
	return b.String()
}

// RouteViewName upper-camels a route name on '-' (events → Events, sneaky-food → SneakyFood).
func RouteViewName(name string) string {
	parts := strings.Split(name, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// RouteViewRelPath is src/custom/pages/<Name>.astro (sneaky layout).
func RouteViewRelPath(name string) string {
	return "src/custom/pages/" + RouteViewName(name) + ".astro"
}

// GenerateRouteView returns the placeholder view written once on route create.
func GenerateRouteView(name string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("// Custom route \"")
	b.WriteString(name)
	b.WriteString("\". Edit freely; this file is yours.\n")
	b.WriteString("import type { ResourceNode } from '@/types/compositorTypes';\n")
	b.WriteString("export interface Props { route: string; slug: string; resources: ResourceNode[] }\n")
	b.WriteString("const { route, slug, resources } = Astro.props;\n")
	b.WriteString("---\n\n")
	b.WriteString("<div data-route=\"")
	b.WriteString(name)
	b.WriteString("\">\n")
	b.WriteString("  <p>Route <code>/")
	b.WriteString(name)
	b.WriteString("/{slug}</code> — {resources.length} resources loaded. Replace this with your markup.</p>\n")
	b.WriteString("</div>\n")
	return b.String()
}

// GenerateShell returns the self-contained route trunk bytes.
// name is the category_slug / URL segment; param1 is the resource slug.
func GenerateShell(name string) string {
	comp := RouteViewName(name)
	importPath := "@/custom/pages/" + comp + ".astro"

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("import ")
	b.WriteString(comp)
	b.WriteString(" from '")
	b.WriteString(importPath)
	b.WriteString("';\n")
	b.WriteString("import { preHealthCheck } from '@/utils/backend';\n")
	b.WriteString("import type { ResourceNode } from '@/types/compositorTypes';\n\n")
	b.WriteString("const tenantId =\n")
	b.WriteString("  Astro.locals.tenant?.id || import.meta.env.PUBLIC_TENANTID || 'default';\n\n")
	b.WriteString("const healthCheckRedirect = await preHealthCheck(tenantId);\n")
	b.WriteString("if (healthCheckRedirect !== undefined) {\n")
	b.WriteString("  return healthCheckRedirect;\n")
	b.WriteString("}\n\n")
	b.WriteString("const { param1 } = Astro.params;\n")
	b.WriteString("const route = `")
	b.WriteString(name)
	b.WriteString("`;\n\n")
	b.WriteString("if (!param1) {\n")
	b.WriteString("  return new Response(null, { status: 404, statusText: 'Not Found' });\n")
	b.WriteString("}\n\n")
	b.WriteString("const CATEGORIES = [`")
	b.WriteString(name)
	b.WriteString("`];\n\n")
	b.WriteString("let resources: ResourceNode[] = [];\n")
	b.WriteString("const goBackend = import.meta.env.PUBLIC_GO_BACKEND || 'http://localhost:8080';\n\n")
	b.WriteString("try {\n")
	b.WriteString("  const resourcesResponse = await fetch(`${goBackend}/api/v1/nodes/resources`, {\n")
	b.WriteString("    method: 'POST',\n")
	b.WriteString("    headers: { 'Content-Type': 'application/json', 'X-Tenant-ID': tenantId },\n")
	b.WriteString("    body: JSON.stringify({ categories: CATEGORIES }),\n")
	b.WriteString("  });\n")
	b.WriteString("  if (resourcesResponse.ok) {\n")
	b.WriteString("    const responseData = await resourcesResponse.json();\n")
	b.WriteString("    if (responseData.resources) resources = responseData.resources;\n")
	b.WriteString("  }\n")
	b.WriteString("} catch (error) {\n")
	b.WriteString("  console.error('Error fetching resources:', error);\n")
	b.WriteString("  return new Response(null, { status: 500, statusText: 'Internal Server Error' });\n")
	b.WriteString("}\n\n")
	b.WriteString("// Optional: redirect or 404 when no resource matches\n")
	b.WriteString("// categorySlug === route and slug === param1.\n")
	b.WriteString("---\n\n")
	b.WriteString("<")
	b.WriteString(comp)
	b.WriteString(" route={route} slug={param1} resources={resources} />\n")
	return b.String()
}

// ShellMatchesGenerated reports whether content is byte-identical to GenerateShell(name).
func ShellMatchesGenerated(content, name string) bool {
	return content == GenerateShell(name)
}
