# TractStack Go - Complete Parallel Codebase Rebuild Plan

## 🎯 Overview
Complete parallel rebuild of ~19,000 lines of Go code across 105 files while preserving exact API contract compatibility.

---

## 🔍 FOUNDATIONAL DEPENDENCIES (Must Be Built First)

## 1. TENANT SYSTEM (Absolute Foundation)

### 📂 Target Structure:
```
internal/infrastructure/tenant/
├── isolation/context.go           # Tenant context management
├── detection/detector.go          # Tenant detection logic  
├── management/config.go           # Tenant configuration
└── management/lifecycle.go        # Tenant activation/provisioning
```

### 📋 Current File Mapping:
```
tenant/context.go → internal/infrastructure/tenant/isolation/context.go
  CHANGE: MINOR - Remove global state access, add DI constructor
  LOGIC: Tenant context creation and management
  LINES: ~150 lines

tenant/detector.go → internal/infrastructure/tenant/detection/detector.go  
  CHANGE: MINOR - Remove global dependencies, interface implementation
  LOGIC: Domain-based tenant detection, validation
  LINES: ~200 lines

tenant/config.go → internal/infrastructure/tenant/management/config.go
  CHANGE: MINOR - Extract file system operations, add interface
  LOGIC: Tenant configuration loading, brand config management
  LINES: ~300 lines

tenant/activation.go → internal/infrastructure/tenant/management/lifecycle.go
tenant/preactivation.go → internal/infrastructure/tenant/management/lifecycle.go
  CHANGE: MAJOR - Combine files, remove global state, add proper error handling
  LOGIC: Tenant lifecycle management, database initialization, activation workflows
  LINES: ~400 lines combined
```

---

## 2. DATABASE ABSTRACTION (Infrastructure Foundation)

### 📂 Target Structure:
```
internal/infrastructure/persistence/
├── database/connection.go         # Database connection management
├── database/adapters.go          # Database operation interfaces
└── repositories/interfaces/      # Repository contracts (interfaces only)
    ├── content.go                 # Content repository interfaces
    ├── analytics.go               # Analytics repository interfaces  
    ├── user.go                    # User repository interfaces
    └── tenant.go                  # Tenant repository interfaces
```

### 📋 Current File Mapping:
```
tenant/database.go → internal/infrastructure/persistence/database/connection.go
  CHANGE: MAJOR - Remove tenant coupling, add connection pooling interfaces
  LOGIC: Database connection management, SQLite/Turso handling
  LINES: ~250 lines

NEW FILE → internal/infrastructure/persistence/database/adapters.go
  CHANGE: NEW - Extract database operation patterns from scattered queries
  LOGIC: Database adapter interfaces, query building utilities
  LINES: ~200 lines (extracted)

NEW FILES → internal/infrastructure/persistence/repositories/interfaces/
  CHANGE: NEW - Extract all loadFromDB method signatures
  SOURCE: All models/content/*.go loadFromDB methods
  LOGIC: Repository interface definitions
  LINES: ~400 lines (extracted from 8 content files)
```

---

## 3. CACHE SYSTEM (Before Repositories)

### 📂 Target Structure:
```
internal/infrastructure/caching/
├── interfaces/                   # Cache contracts
│   ├── content.go                # Content cache interface
│   ├── fragments.go              # Fragment cache interface
│   ├── analytics.go              # Analytics cache interface
│   └── sessions.go               # Session cache interface
├── manager/manager.go            # Cache coordination
├── stores/                       # Cache implementations
│   ├── content.go                # Content cache store
│   ├── fragments.go              # HTML fragment cache store
│   ├── analytics.go              # Analytics cache store
│   └── sessions.go               # Session cache store
├── invalidation/coordinator.go   # Cache invalidation orchestration
└── warming/coordinator.go        # Cache warming orchestration
```

### 📋 Current File Mapping:
```
cache/interface.go → internal/infrastructure/caching/interfaces/
  CHANGE: MINOR - Split into focused interfaces per domain
  LOGIC: Cache interface definitions
  LINES: ~300 lines → split into 4 files

cache/manager.go → internal/infrastructure/caching/manager/manager.go
  CHANGE: MAJOR - Remove global singleton, add DI constructor
  LOGIC: Cache manager coordination, tenant isolation
  LINES: ~500 lines

cache/content/*.go (8 files) → internal/infrastructure/caching/stores/content.go
  CHANGE: MAJOR - Consolidate 8 files, remove business logic  
  SOURCE FILES:
    - cache/content/beliefs.go (~200 lines)
    - cache/content/epinets.go (~150 lines)
    - cache/content/imagefiles.go (~150 lines)
    - cache/content/menus.go (~200 lines)
    - cache/content/panes.go (~200 lines)
    - cache/content/resources.go (~200 lines)
    - cache/content/storyfragments.go (~250 lines)
    - cache/content/tractstacks.go (~150 lines)
  LOGIC: Content caching operations
  LINES: ~1500 lines → consolidated to ~800

cache/html.go → internal/infrastructure/caching/stores/fragments.go
  CHANGE: MINOR - Interface implementation, remove globals
  LOGIC: HTML fragment caching with dependency tracking
  LINES: ~300 lines

cache/analytics.go → internal/infrastructure/caching/stores/analytics.go
cache/analytics_adapters.go → internal/infrastructure/caching/stores/analytics.go
cache/analytics_interfaces.go → internal/infrastructure/caching/interfaces/analytics.go
  CHANGE: MAJOR - Consolidate analytics caching, separate interfaces
  LOGIC: Analytics cache operations, hourly binning
  LINES: ~600 lines combined

NEW FILE → internal/infrastructure/caching/invalidation/coordinator.go
  CHANGE: NEW - Extract invalidation logic from handlers
  SOURCE: Invalidation cascades from api/*_handlers.go
  LOGIC: Cache invalidation orchestration
  LINES: ~300 lines (extracted)

services/cache_warmer.go → internal/infrastructure/caching/warming/coordinator.go
warming/warming.go → internal/infrastructure/caching/warming/coordinator.go
  CHANGE: MAJOR - **CRITICAL ARCHITECTURAL FIX** - Combine files, eliminate direct DB access anti-pattern
  ANTI-PATTERN: cache_warmer.go intentionally bypasses cache-first service layer with direct DB queries
  CRITICAL ISSUE: Methods like getEpinets() and getContentItems() ignore existing cache-aware services
  LOGIC: Cache warming strategies, background processing - MUST use repository interfaces only
  LINES: ~400 lines combined → ~300 lines (eliminate DB side-channel)

cache/utils.go → internal/infrastructure/caching/manager/utils.go
cache/validation.go → internal/infrastructure/caching/manager/validation.go
cache/warming_lock.go → internal/infrastructure/caching/warming/locks.go
  CHANGE: MINOR - Move to appropriate subdirectories
  LOGIC: Cache utilities, validation, locking mechanisms
  LINES: ~200 lines total
```

---

## 4. REPOSITORY IMPLEMENTATIONS (Data Access Layer)

### 📂 Target Structure:
```
internal/infrastructure/persistence/repositories/
├── content/                      # Content data access
│   ├── tractstack.go            # TractStack repository
│   ├── storyfragment.go         # StoryFragment repository  
│   ├── pane.go                  # Pane repository
│   ├── menu.go                  # Menu repository
│   ├── resource.go              # Resource repository
│   ├── belief.go                # Belief repository
│   ├── epinet.go                # Epinet repository
│   ├── imagefile.go             # ImageFile repository
│   └── contentmap.go            # Content map repository
├── analytics/                   # Analytics data access
│   ├── events.go                # Event storage/retrieval
│   └── metrics.go               # Metrics computation
├── user/                        # User data access
│   ├── session.go               # Session management
│   ├── fingerprint.go           # Fingerprint management
│   └── visit.go                 # Visit tracking
└── tenant/tenant.go             # Tenant data access
```

### 📋 Current File Mapping:
```
models/content/tractstacks.go → internal/infrastructure/persistence/repositories/content/tractstack.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: TractStack data access operations
  LINES: ~300 lines → split (100 entity + 200 repository)

models/content/storyfragments.go → internal/infrastructure/persistence/repositories/content/storyfragment.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: StoryFragment data access operations  
  LINES: ~400 lines → split (150 entity + 250 repository)

models/content/panes.go → internal/infrastructure/persistence/repositories/content/pane.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: Pane data access operations
  LINES: ~500 lines → split (200 entity + 300 repository)

models/content/menus.go → internal/infrastructure/persistence/repositories/content/menu.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: Menu data access operations
  LINES: ~400 lines → split (150 entity + 250 repository)

models/content/resources.go → internal/infrastructure/persistence/repositories/content/resource.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: Resource data access operations
  LINES: ~450 lines → split (200 entity + 250 repository)

models/content/beliefs.go → internal/infrastructure/persistence/repositories/content/belief.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: Belief data access operations
  LINES: ~350 lines → split (150 entity + 200 repository)

models/content/epinets.go → internal/infrastructure/persistence/repositories/content/epinet.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: Epinet data access operations
  LINES: ~300 lines → split (150 entity + 150 repository)

models/content/imagefiles.go → internal/infrastructure/persistence/repositories/content/imagefile.go
  CHANGE: MAJOR - Extract loadFromDB methods, implement cache-first pattern
  LOGIC: ImageFile data access operations
  LINES: ~250 lines → split (100 entity + 150 repository)

api/content_handlers.go → internal/infrastructure/persistence/repositories/content/contentmap.go
  CHANGE: MAJOR - Extract BuildFullContentMapFromDB function
  LOGIC: Content map UNION query, content discovery
  LINES: ~200 lines (extracted from handler)

NEW FILE → internal/infrastructure/persistence/repositories/analytics/events.go
  CHANGE: NEW - Extract analytics queries from services and handlers
  SOURCE: events/*.go database queries, analytics_handlers.go queries
  LOGIC: Analytics event storage and retrieval
  LINES: ~300 lines (extracted)

NEW FILE → internal/infrastructure/persistence/repositories/analytics/metrics.go
  CHANGE: NEW - Extract metrics computation queries
  SOURCE: services/analytics.go, services/cache_warmer.go database operations
  LOGIC: Analytics metrics computation, aggregation queries
  LINES: ~400 lines (extracted)

NEW FILE → internal/infrastructure/persistence/repositories/user/session.go
  CHANGE: NEW - Extract session management queries
  SOURCE: api/visit_handlers.go, models/models.go
  LOGIC: Session creation, fingerprinting, user state management
  LINES: ~200 lines (extracted)

NEW FILE → internal/infrastructure/persistence/repositories/user/fingerprint.go
  CHANGE: NEW - Extract fingerprint management queries
  SOURCE: api/visit_handlers.go, events/belief_processor.go
  LOGIC: Fingerprint creation, belief state management
  LINES: ~150 lines (extracted)

NEW FILE → internal/infrastructure/persistence/repositories/user/visit.go
  CHANGE: NEW - Extract visit tracking queries
  SOURCE: api/visit_handlers.go
  LOGIC: Visit creation, tracking, campaign attribution
  LINES: ~100 lines (extracted)

NEW FILE → internal/infrastructure/persistence/repositories/tenant/tenant.go
  CHANGE: NEW - Extract tenant management queries
  SOURCE: tenant/*.go database operations, api/multi_tenant_handlers.go
  LOGIC: Tenant provisioning, activation, configuration persistence
  LINES: ~200 lines (extracted)
```

---

## 5. DOMAIN ENTITIES (Pure Data Structures)

### 📂 Target Structure:
```
internal/domain/entities/
├── content/                     # Content domain objects
│   ├── tractstack.go           # TractStack entity
│   ├── storyfragment.go        # StoryFragment entity
│   ├── pane.go                 # Pane entity
│   ├── menu.go                 # Menu entity
│   ├── resource.go             # Resource entity
│   ├── belief.go               # Belief entity
│   ├── epinet.go              # Epinet entity
│   ├── imagefile.go           # ImageFile entity
│   ├── contentmap.go          # Content map structures
│   └── rendering.go           # HTML rendering structures
├── user/                       # User domain objects
│   ├── session.go             # Session entity
│   ├── fingerprint.go         # Fingerprint entity
│   └── visit.go               # Visit entity
├── analytics/                  # Analytics domain objects
│   └── analytics.go           # Analytics entities
└── tenant/                     # Tenant domain objects
    └── tenant.go              # Tenant entity
```

### 📋 Current File Mapping:
```
models/content/tractstacks.go → internal/domain/entities/content/tractstack.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: TractStack entity definition, validation rules
  LINES: ~300 lines → ~100 lines (entity only)

models/content/storyfragments.go → internal/domain/entities/content/storyfragment.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: StoryFragment entity definition, validation rules
  LINES: ~400 lines → ~150 lines (entity only)

models/content/panes.go → internal/domain/entities/content/pane.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: Pane entity definition, validation rules
  LINES: ~500 lines → ~200 lines (entity only)

models/content/menus.go → internal/domain/entities/content/menu.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: Menu entity definition, validation rules
  LINES: ~400 lines → ~150 lines (entity only)

models/content/resources.go → internal/domain/entities/content/resource.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: Resource entity definition, validation rules
  LINES: ~450 lines → ~200 lines (entity only)

models/content/beliefs.go → internal/domain/entities/content/belief.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: Belief entity definition, validation rules
  LINES: ~350 lines → ~150 lines (entity only)

models/content/epinets.go → internal/domain/entities/content/epinet.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: Epinet entity definition, validation rules
  LINES: ~300 lines → ~150 lines (entity only)

models/content/imagefiles.go → internal/domain/entities/content/imagefile.go
  CHANGE: MAJOR - Extract only data structures, remove service methods
  LOGIC: ImageFile entity definition, validation rules
  LINES: ~250 lines → ~100 lines (entity only)

models/content_map.go → internal/domain/entities/content/contentmap.go
  CHANGE: MINOR - Move to domain layer, add validation methods
  LOGIC: Content map structures, content discovery types
  LINES: ~200 lines

models/html.go → internal/domain/entities/content/rendering.go
  CHANGE: MINOR - Move to domain layer, focus on data structures
  LOGIC: HTML rendering data structures, node definitions
  LINES: ~300 lines

models/models.go → internal/domain/entities/user/session.go
models/models.go → internal/domain/entities/user/fingerprint.go
models/models.go → internal/domain/entities/user/visit.go
  CHANGE: MAJOR - Split large file into focused entities
  LOGIC: User session, fingerprint, visit entities
  LINES: ~400 lines → split into 3 files (~130 each)

models/analytics.go → internal/domain/entities/analytics/analytics.go
  CHANGE: MINOR - Move to domain layer, focus on data structures
  LOGIC: Analytics entities, event structures, metrics definitions
  LINES: ~300 lines

models/multi_tenant.go → internal/domain/entities/tenant/tenant.go
  CHANGE: MINOR - Move to domain layer, add domain validation
  LOGIC: Tenant entity, provisioning structures
  LINES: ~150 lines

models/orphan.go → internal/domain/entities/content/dependencies.go
  CHANGE: MINOR - Move to domain layer, rename for clarity
  LOGIC: Dependency analysis structures, orphan detection data
  LINES: ~100 lines
```

---

## 6. DOMAIN SERVICES (Pure Business Logic)

### 📂 Target Structure:
```
internal/domain/services/
├── belief_evaluation.go        # Belief evaluation logic
├── belief_registry.go          # Belief registry logic
├── content_validation.go       # Content validation rules
├── personalization.go          # Personalization logic
└── dependency_analysis.go      # Orphan analysis logic
```

### 📋 Current File Mapping:
```
services/belief_evaluation.go → internal/domain/services/belief_evaluation.go
  CHANGE: MINOR - Remove dependencies on infrastructure, pure business logic
  LOGIC: Belief-based content filtering, visibility evaluation
  LINES: ~400 lines

services/belief_registry.go → internal/domain/services/belief_registry.go
  CHANGE: MINOR - Remove cache dependencies, pure registry logic
  LOGIC: Storyfragment belief requirement management
  LINES: ~300 lines

NEW FILE → internal/domain/services/content_validation.go
  CHANGE: NEW - Extract validation logic from models and handlers
  SOURCE: Validation logic scattered across api/*_handlers.go
  LOGIC: Content validation rules, schema validation, business rules
  LINES: ~250 lines (extracted)

NEW FILE → internal/domain/services/personalization.go
  CHANGE: NEW - Extract personalization logic from handlers
  SOURCE: api/pane_fragment_handler.go personalization logic
  LOGIC: Content personalization rules, belief-based filtering
  LINES: ~200 lines (extracted)

services/orphan_analysis.go → internal/domain/services/dependency_analysis.go
  CHANGE: MAJOR - Remove direct database access, use repository interfaces
  LOGIC: Content dependency analysis, orphan detection algorithms
  LINES: ~500 lines → ~300 lines (remove DB code)
```

---

## 7. APPLICATION SERVICES (Orchestration)

### 📂 Target Structure:
```
internal/application/services/
├── content.go                  # Content management orchestration
├── fragments.go                # Fragment delivery orchestration  
├── analytics.go                # Analytics orchestration
├── auth.go                     # Authentication orchestration
├── admin.go                    # Administrative operations
└── messaging.go                # Real-time messaging orchestration
```

### 📋 Current File Mapping:
```
NEW FILE → internal/application/services/content.go
  CHANGE: NEW - Extract business logic from content handlers
  SOURCE: api/content_handlers.go, api/*_handlers.go (content-related)
  LOGIC: Content CRUD orchestration, cache invalidation coordination
  LINES: ~600 lines (extracted from 10+ handler files)

NEW FILE → internal/application/services/fragments.go
  CHANGE: NEW - Extract fragment logic from handlers
  SOURCE: api/pane_fragment_handler.go business logic
  LOGIC: HTML fragment generation orchestration, personalization coordination
  LINES: ~400 lines (extracted)

services/analytics.go → internal/application/services/analytics.go
  CHANGE: MAJOR - Remove direct cache access, use repository interfaces
  LOGIC: Analytics computation orchestration, Sankey diagram generation
  LINES: ~800 lines → ~600 lines (remove infrastructure code)

NEW FILE → internal/application/services/auth.go
  CHANGE: NEW - **CRITICAL COMPLEXITY** - Extract authentication logic from handlers
  SOURCE: api/auth_handlers.go, api/visit_handlers.go, api/profile_handlers.go business logic
  COMPLEXITY WARNING: This service will orchestrate the most complex workflows in the system:
    • Multi-step user authentication flows
    • Session lifecycle management across multiple cache stores
    • Fingerprint-to-lead association logic
    • Visit tracking and campaign attribution
    • JWT token lifecycle (generation, validation, refresh)
    • SSE connection authorization and management
  REPOSITORY DEPENDENCIES: SessionRepository, FingerprintRepository, LeadRepository, VisitRepository
  DOMAIN SERVICE DEPENDENCIES: AuthenticationService, SessionService
  LOGIC: Session management, authentication workflows, JWT handling - **HIGHEST RISK REFACTOR**
  LINES: ~500 lines (complex orchestration logic)

NEW FILE → internal/application/services/admin.go
  CHANGE: NEW - Extract administrative logic from handlers
  SOURCE: api/orphan_handlers.go, api/multi_tenant_handlers.go business logic
  LOGIC: Administrative operations, tenant management, system monitoring
  LINES: ~300 lines (extracted)

services/belief_broadcaster.go → internal/application/services/messaging.go
  CHANGE: MAJOR - Expand to handle all real-time messaging
  SOURCE: SSE logic from api/visit_handlers.go
  LOGIC: Real-time messaging orchestration, SSE coordination, event broadcasting
  LINES: ~300 lines + extracted SSE logic (~200 lines) = ~500 lines
```

---

## 8. TEMPLATE SYSTEM (HTML Generation)

### 📂 Target Structure:
```
internal/presentation/templates/
├── engine/                     # Template engine
│   ├── generator.go           # HTML generation
│   ├── renderer.go            # Rendering orchestration
│   └── parser.go              # Node parsing
├── components/                # Reusable components
│   ├── nodes/                 # Node renderers
│   └── widgets/               # Widget renderers
└── assets/css.go              # CSS utilities
```

### 📋 Current File Mapping:
```
html/generator.go → internal/presentation/templates/engine/generator.go
  CHANGE: MINOR - Remove global cache access, add DI
  LOGIC: HTML generation engine, node processing
  LINES: ~400 lines

html/renderer.go → internal/presentation/templates/engine/renderer.go
  CHANGE: MINOR - Remove global dependencies, interface implementation
  LOGIC: Rendering orchestration, template coordination
  LINES: ~300 lines

html/node_parser.go → internal/presentation/templates/engine/parser.go
  CHANGE: MINOR - Remove global dependencies, pure parsing logic
  LOGIC: Node parsing, optionsPayload processing
  LINES: ~250 lines

html/css.go → internal/presentation/templates/assets/css.go
  CHANGE: COPY - Move to assets subdirectory
  LOGIC: CSS utility functions
  LINES: ~100 lines

html/templates/bgPaneWrapper.go → internal/presentation/templates/components/nodes/bgPaneWrapper.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Background pane wrapper rendering
  LINES: ~150 lines

html/templates/emptyNode.go → internal/presentation/templates/components/nodes/emptyNode.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Empty node rendering
  LINES: ~50 lines

html/templates/markdown.go → internal/presentation/templates/components/nodes/markdown.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Markdown node rendering
  LINES: ~100 lines

html/templates/nodeA.go → internal/presentation/templates/components/nodes/nodeA.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Anchor node rendering
  LINES: ~80 lines

html/templates/nodeBasicTag.go → internal/presentation/templates/components/nodes/nodeBasicTag.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Basic HTML tag rendering
  LINES: ~60 lines

html/templates/nodeButton.go → internal/presentation/templates/components/nodes/nodeButton.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Button node rendering
  LINES: ~80 lines

html/templates/nodeImage.go → internal/presentation/templates/components/nodes/nodeImage.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Image node rendering
  LINES: ~120 lines

html/templates/nodeText.go → internal/presentation/templates/components/nodes/nodeText.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Text node rendering
  LINES: ~70 lines

html/templates/tagElement.go → internal/presentation/templates/components/nodes/tagElement.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Tag element rendering
  LINES: ~90 lines

html/templates/widget.go → internal/presentation/templates/components/widgets/widget.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Widget dispatcher
  LINES: ~200 lines

html/templates/widgets/belief.go → internal/presentation/templates/components/widgets/belief.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Belief widget rendering
  LINES: ~150 lines

html/templates/widgets/bunny.go → internal/presentation/templates/components/widgets/bunny.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Bunny video widget rendering
  LINES: ~80 lines

html/templates/widgets/identifyAs.go → internal/presentation/templates/components/widgets/identifyAs.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: IdentifyAs widget rendering
  LINES: ~120 lines

html/templates/widgets/resource.go → internal/presentation/templates/components/widgets/resource.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Resource widget rendering
  LINES: ~60 lines

html/templates/widgets/shared.go → internal/presentation/templates/components/widgets/shared.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Shared widget utilities
  LINES: ~100 lines

html/templates/widgets/signup.go → internal/presentation/templates/components/widgets/signup.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Signup widget rendering
  LINES: ~100 lines

html/templates/widgets/toggle.go → internal/presentation/templates/components/widgets/toggle.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Toggle widget rendering
  LINES: ~90 lines

html/templates/widgets/youtube.go → internal/presentation/templates/components/widgets/youtube.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: YouTube widget rendering
  LINES: ~70 lines
```

---

## 9. HTTP HANDLERS (Thin API Layer)

### 📂 Target Structure:
```
internal/presentation/http/
├── handlers/                  # HTTP handlers
│   ├── content.go            # Content endpoints
│   ├── fragments.go          # Fragment endpoints  
│   ├── analytics.go          # Analytics endpoints
│   ├── auth.go               # Auth endpoints
│   ├── admin.go              # Admin endpoints
│   ├── config.go             # Config endpoints
│   ├── tenant.go             # Multi-tenant endpoints
│   └── system.go             # Health/system endpoints
├── middleware/               # HTTP middleware
├── routes/routes.go          # Route definitions
└── dto/                      # Request/response DTOs
    ├── requests.go           # Request structures
    └── responses.go          # Response structures
```

### 📋 Current File Mapping:
```
api/content_handlers.go → internal/presentation/http/handlers/content.go
api/belief_handlers.go → internal/presentation/http/handlers/content.go
api/menu_handlers.go → internal/presentation/http/handlers/content.go
api/resource_handlers.go → internal/presentation/http/handlers/content.go
api/pane_handlers.go → internal/presentation/http/handlers/content.go
api/storyfragment_handlers.go → internal/presentation/http/handlers/content.go
api/tractstack_handlers.go → internal/presentation/http/handlers/content.go
api/epinets_handlers.go → internal/presentation/http/handlers/content.go
api/imagefile_handlers.go → internal/presentation/http/handlers/content.go
  CHANGE: MAJOR - Consolidate 9 files, extract business logic to services
  LOGIC: All content CRUD endpoints, thin HTTP layer only
  LINES: ~3000 lines → ~1500 lines (remove business logic)

api/pane_fragment_handler.go → internal/presentation/http/handlers/fragments.go
  CHANGE: MAJOR - Extract personalization logic to services
  LOGIC: HTML fragment delivery endpoints
  LINES: ~500 lines → ~200 lines (remove business logic)

api/analytics_handlers.go → internal/presentation/http/handlers/analytics.go
  CHANGE: MAJOR - Extract analytics logic to services
  LOGIC: Analytics endpoints, dashboard data
  LINES: ~600 lines → ~300 lines (remove business logic)

api/auth_handlers.go → internal/presentation/http/handlers/auth.go
api/visit_handlers.go → internal/presentation/http/handlers/auth.go
api/profile_handlers.go → internal/presentation/http/handlers/auth.go
  CHANGE: MAJOR - **HIGHEST COMPLEXITY** - Consolidate auth-related handlers, extract tangled business logic
  COMPLEXITY WARNING: These are not simple endpoints - they contain highly complex stateful workflows:
    • New user/session creation with multiple validation paths
    • Existing user authentication (password + encrypted credentials)
    • Fingerprint creation and lead linking
    • Visit creation and fingerprint association
    • Multi-table database operations (leads, fingerprints, visits)
    • Multi-cache operations (SessionStates, VisitStates, FingerprintStates)
    • JWT token generation and validation
    • SSE connection lifecycle management
  REPOSITORY DEPENDENCIES: Will require SessionRepository, FingerprintRepository, LeadRepository, VisitRepository
  SERVICE DEPENDENCIES: AuthService, SessionService, FingerprintService
  LOGIC: Authentication, session management, profile endpoints - **SCHEDULE AS CRITICAL PATH ITEM**
  LINES: ~800 lines → ~400 lines (extract to multiple services/repositories)

api/orphan_handlers.go → internal/presentation/http/handlers/admin.go
  CHANGE: MAJOR - Extract orphan analysis logic to services
  LOGIC: Administrative endpoints, system management
  LINES: ~200 lines → ~100 lines (remove business logic)

api/advanced_handlers.go → internal/presentation/http/handlers/config.go
api/brand_handlers.go → internal/presentation/http/handlers/config.go
  CHANGE: MAJOR - Consolidate config handlers, extract logic to services
  LOGIC: Configuration management endpoints
  LINES: ~600 lines → ~300 lines (remove business logic)

api/multi_tenant_handlers.go → internal/presentation/http/handlers/tenant.go
  CHANGE: MAJOR - Extract tenant management logic to services
  LOGIC: Multi-tenant management endpoints
  LINES: ~400 lines → ~200 lines (remove business logic)

api/handlers.go → internal/presentation/http/handlers/system.go
  CHANGE: MINOR - Health and system endpoints
  LOGIC: Health checks, system status
  LINES: ~100 lines

api/middleware.go → internal/presentation/http/middleware/middleware.go
  CHANGE: MINOR - Remove global dependencies, add DI
  LOGIC: HTTP middleware, tenant detection, CORS
  LINES: ~200 lines

NEW FILE → internal/presentation/http/middleware/cors.go
  CHANGE: NEW - Extract complex CORS and security configuration from main.go
  SOURCE: main.go CORS configuration (r.Use(cors.New(...))), domain validation logic
  SECURITY CRITICAL: Domain whitelist validation, CORS policy enforcement
  LOGIC: CORS configuration, domain validation middleware, security headers
  LINES: ~150 lines (extracted from main.go)

NEW FILE → internal/presentation/http/routes/routes.go
  CHANGE: NEW - Extract route definitions and middleware setup from main.go
  SOURCE: main.go route registration, middleware application
  SECURITY DEPENDENCIES: Must integrate with cors.go for domain validation
  LOGIC: HTTP route definitions, endpoint registration, middleware orchestration
  LINES: ~350 lines (extracted + middleware integration)

api/helpers.go → internal/presentation/http/dto/requests.go
api/helpers.go → internal/presentation/http/dto/responses.go
  CHANGE: MAJOR - Split into request/response DTOs
  LOGIC: HTTP request/response structures, validation
  LINES: ~200 lines → split into 2 files
```

---

## 10. EVENT PROCESSING & MESSAGING

### 📂 Target Structure:
```
internal/infrastructure/messaging/
├── events/                    # Event processing
│   ├── processor.go           # Event coordination
│   ├── analytics.go           # Analytics event handling
│   └── beliefs.go             # Belief event handling
└── sse/                       # Server-Sent Events
    ├── broadcaster.go         # SSE broadcasting
    └── connections.go         # Connection management
```

### 📋 Current File Mapping:
```
events/event_processor.go → internal/infrastructure/messaging/events/processor.go
  CHANGE: MAJOR - Remove direct database access, use repository interfaces
  LOGIC: Event processing coordination, event routing
  LINES: ~200 lines → ~150 lines (remove DB code)

events/analytics_processor.go → internal/infrastructure/messaging/events/analytics.go
  CHANGE: MAJOR - Remove direct database access, use repository interfaces
  LOGIC: Analytics event processing, pane/storyfragment tracking
  LINES: ~300 lines → ~200 lines (remove DB code)

events/belief_processor.go → internal/infrastructure/messaging/events/beliefs.go
  CHANGE: MAJOR - Remove direct database access, use repository interfaces
  LOGIC: Belief event processing, user state management
  LINES: ~400 lines → ~250 lines (remove DB code)

NEW FILE → internal/infrastructure/messaging/sse/broadcaster.go
  CHANGE: NEW - Extract SSE logic from visit handlers
  SOURCE: SSE broadcasting logic from api/visit_handlers.go
  LOGIC: Server-Sent Events broadcasting, real-time messaging
  LINES: ~300 lines (extracted)

NEW FILE → internal/infrastructure/messaging/sse/connections.go
  CHANGE: NEW - Extract SSE connection management
  SOURCE: Connection management from api/visit_handlers.go, models/models.go SSE types
  LOGIC: SSE connection lifecycle, session management
  LINES: ~200 lines (extracted)
```

---

## 11. EXTERNAL INTEGRATIONS

### 📂 Target Structure:
```
internal/infrastructure/external/
└── email/                     # Email services
    ├── client.go              # Email client
    └── templates/             # Email templates
        ├── activation.go      # Activation email
        ├── components.go      # Email components
        ├── layout.go          # Email layout
        └── sandbox.go         # Sandbox email
```

### 📋 Current File Mapping:
```
email/client.go → internal/infrastructure/external/email/client.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Email client implementation, SMTP handling
  LINES: ~200 lines

email/templates/activation.go → internal/infrastructure/external/email/templates/activation.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Tenant activation email template
  LINES: ~100 lines

email/templates/components.go → internal/infrastructure/external/email/templates/components.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Email template components
  LINES: ~150 lines

email/templates/layout.go → internal/infrastructure/external/email/templates/layout.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Email layout template
  LINES: ~100 lines

email/templates/sandbox.go → internal/infrastructure/external/email/templates/sandbox.go
  CHANGE: COPY - Direct move with import path updates
  LOGIC: Sandbox environment email template
  LINES: ~80 lines
```

---

## 12. SHARED PACKAGES

### 📂 Target Structure:
```
pkg/
├── utils/                     # Shared utilities
│   ├── crypto/crypto.go       # Cryptographic utilities
│   ├── images/                # Image processing
│   ├── lisp/                  # Lisp parsing
│   └── analytics/analytics.go # Analytics utilities
└── config/                    # Configuration management
    └── defaults.go            # Default configuration
```

### 📋 Current File Mapping:
```
utils/crypto.go → pkg/utils/crypto/crypto.go
  CHANGE: COPY - Direct move to pkg structure
  LOGIC: Cryptographic utilities, JWT handling
  LINES: ~150 lines

utils/images/canvas.go → pkg/utils/images/canvas.go
utils/images/multi_size.go → pkg/utils/images/multi_size.go
utils/images/processor.go → pkg/utils/images/processor.go
  CHANGE: COPY - Direct move to pkg structure
  LOGIC: Image processing, canvas operations, multi-size generation
  LINES: ~400 lines total

utils/lisp/lexer.go → pkg/utils/lisp/lexer.go
utils/lisp/parser.go → pkg/utils/lisp/parser.go
  CHANGE: COPY - Direct move to pkg structure
  LOGIC: Lisp parsing for ActionLisp expressions
  LINES: ~300 lines total

utils/analytics.go → pkg/utils/analytics/analytics.go
  CHANGE: COPY - Direct move to pkg structure
  LOGIC: Analytics utility functions, time formatting
  LINES: ~200 lines

config/defaults.go → pkg/config/defaults.go
  CHANGE: COPY - Direct move to pkg structure
  LOGIC: Default configuration values, environment loading
  LINES: ~150 lines
```

---

## 13. APPLICATION ENTRY POINT (Final Assembly)

### 📂 Target Structure:
```
cmd/server/main.go              # Application startup with DI container
```

### 📋 Current File Mapping:
```
main.go → cmd/server/main.go
  CHANGE: MAJOR - Complete rewrite with dependency injection container
  LOGIC: Application startup, DI container setup, server initialization
  LINES: ~400 lines → ~300 lines (clean DI setup)
```

---

## 📊 COMPLETE REBUILD SUMMARY

### 📈 File Count Analysis:
```
TOTAL CURRENT FILES: 105 Go files (19,013 lines)

FILES TO COPY (MINOR/NO CHANGES): 45 files (~4,500 lines)
- Template files: 27 files (~2,500 lines)
- Email templates: 5 files (~630 lines)  
- Utilities: 8 files (~1,200 lines)
- Root config files: 5 files (~170 lines)

FILES REQUIRING MAJOR REWORK: 60 files (~14,513 lines)
- API handlers: 20 files (~6,000 lines → ~3,000 lines after logic extraction)
- Models/Content: 8 files (~3,000 lines → split into entities + repositories)
- Cache system: 17 files (~3,500 lines → ~2,000 lines after consolidation)
- Core models: 6 files (~1,500 lines → split by domain)
- Services/Events: 9 files (~2,513 lines → ~2,000 lines after cleanup)

NEW FILES TO CREATE: ~25 files (~3,000 lines)
- Repository interfaces: ~400 lines
- Application services: ~1,500 lines  
- Infrastructure adapters: ~600 lines
- Domain services: ~500 lines
```

### 🔄 Rework Intensity:
```
COPY/MINOR (42%): 45 files - Direct copy or minimal import path changes
MAJOR REWORK (58%): 60 files - Significant architectural changes required
```

### ⚡ Critical Success Factors:

#### ✅ **API Contract Preservation**
- All 60+ endpoints maintain exact JSON responses
- HTTP status codes and error messages unchanged
- Authentication patterns preserved
- Caching behavior identical from client perspective

#### 🏗️ **Architecture Principles**
- **Dependency Inversion**: Repository interfaces in domain, implementations in infrastructure
- **Single Responsibility**: Each layer has one clear purpose
- **Interface Segregation**: Small, focused interfaces per domain
- **No Circular Dependencies**: Clean unidirectional dependency flow

#### 🚀 **Performance Preservation**
- Cache-first patterns maintained in repositories
- HTML fragment caching performance unchanged
- Analytics hourly aggregation continues working
- Session/belief tracking performance maintained
- Multi-tenant isolation performance preserved

#### 🔧 **Migration Strategy**
1. **Build complete parallel codebase** using new structure
2. **Implement all interfaces** with same business logic
3. **Test API compatibility** endpoint by endpoint
4. **Single atomic switch** from old to new codebase
5. **Preserve exact functionality** while gaining clean architecture

---

## 🎯 BUILD SEQUENCE DEPENDENCIES

### **Phase 1: Infrastructure Foundation (Days 1-3)**
```
1. Tenant system (isolation, detection, management)
2. Database abstraction (connection, adapters, interfaces)  
3. Cache system (interfaces, stores, coordination)
4. Repository implementations (data access layer)
```

### **Phase 2: Business Logic (Days 4-5)**
```
5. Domain entities (pure data structures)
6. Domain services (pure business logic)
7. Application services (orchestration layer)
```

### **Phase 3: Presentation Layer (Days 6-7)**
```
8. Template system (HTML generation)
9. HTTP handlers (thin API layer)
10. Event processing system
11. External integrations
```

### **Phase 4: Final Assembly (Day 8-9)**
```
12. Dependency injection container
13. Application startup
14. Integration testing
15. Performance validation
```

---

## ⚠️ CRITICAL COMPLEXITY WARNINGS

### **🚨 Highest Risk Components (Schedule First)**

#### **1. Authentication System Refactor**
- **Files**: api/auth_handlers.go, api/visit_handlers.go, api/profile_handlers.go
- **Risk Level**: **CRITICAL** - Most complex stateful workflows in codebase
- **Dependencies**: 4+ new repositories, 3+ new services
- **Timeline**: **2 full days minimum** - Do not underestimate

#### **2. Cache Warmer Anti-Pattern Fix**
- **Files**: services/cache_warmer.go, warming/warming.go  
- **Risk Level**: **HIGH** - Currently bypasses all architectural layers
- **Critical Fix**: Must eliminate direct database access completely
- **Timeline**: **1 full day** - Requires careful repository integration

#### **3. Security Configuration Migration**
- **Files**: main.go CORS/middleware, api/middleware.go
- **Risk Level**: **MEDIUM** - Security-critical configuration
- **Requirements**: Domain validation, CORS policies must remain identical
- **Timeline**: **Half day** - Test thoroughly

---

## 🎯 REVISED BUILD SEQUENCE (Risk-Adjusted)

### **Phase 1: Infrastructure Foundation (Days 1-3)**
```
1. Tenant system (isolation, detection, management)
2. Database abstraction (connection, adapters, interfaces)  
3. Cache system (interfaces, stores, coordination)
4. Repository implementations (data access layer)
```

### **Phase 2: Critical Risk Components (Days 4-6)**
```
5. Authentication repositories and services (CRITICAL - 2 days)
6. Cache warmer refactor (HIGH RISK - 1 day)
```

### **Phase 3: Business Logic & Presentation (Days 7-8)**
```
7. Domain entities and services
8. Application services (non-auth)
9. Template system and HTTP handlers
10. Security middleware migration
```





1. Git Branching Strategy
First, we will create the proper branch from main. All subsequent work will be committed here.
Generated bash
# Ensure you are on the main branch and have the latest changes
git checkout main
git pull origin main

# Create the new 'proper' branch for our work
git checkout -b proper
Use code with caution.
Bash
2. The Parallel Rebuild Strategy: "Building Around" the Old Code
This is the core of the strategy. We will build the entire new application structure within the internal/ and pkg/ directories inside the proper branch, leaving the existing root-level packages (api/, services/, models/, etc.) untouched for now.
The Goal: We will reach a point where two complete applications live side-by-side in the same codebase: the old one and the new one.
The Switch: The final step is to rewrite the application's entry point (main.go) to bootstrap the new, properly structured application from the internal/ directory. Once that new entry point is working, we perform the final cleanup of all the old root-level packages.
Phase-by-Phase Execution Plan with Cumulative Cleanup (on the proper branch)
Here is the dry run, detailing what will be built and what old code becomes obsolete in each phase.
Phase 1: Infrastructure - Tenant System
New Directories to Create: internal/infrastructure/tenant/{isolation,detection,management}
Actions: Create the new DI-friendly Context, Detector, Config, and Lifecycle management files in the new directories. This code will be a refactored version of the logic in the old tenant/ directory.
Cumulative Cleanup (What becomes obsolete): The entire tenant/ directory. We will not delete it yet, as the old main.go and api/ handlers still depend on it. It is now considered "legacy code" to be removed in the final phase.
Phase 2: Infrastructure - Database & Repositories
New Directories to Create: internal/infrastructure/persistence/{database,repositories}
Actions:
Create the new database/connection.go by refactoring tenant/database.go.
Create the new repository interfaces in repositories/interfaces/.
Create the new repository implementations in repositories/content/, repositories/analytics/, etc., by extracting all database query logic from the old models/content/*.go files.
Cumulative Cleanup (What becomes obsolete): All the methods (e.g., loadFromDB, GetBySlug) inside the files in models/content/. The struct definitions themselves are still needed by the old code, but their data-access responsibilities are now fully replaced.
Phase 3: Domain Entities
New Directories to Create: internal/domain/entities/{content,user,analytics,tenant}
Actions: Copy all struct definitions from the models/ directory into the new, properly structured entities/ directories. Remove all methods, leaving only pure data structures.
Cumulative Cleanup (What becomes obsolete): The entire models/ directory is now fully superseded. The old code still imports from it, but all of its definitions now have a "proper" counterpart in the internal/domain/entities/ directory.
Phase 4: Domain Services
New Directories to Create: internal/domain/services/
Actions: Create the new, pure business logic services (belief_evaluation.go, dependency_analysis.go, etc.). This involves refactoring the old services/ files to remove any database or cache dependencies, making them operate only on domain entities and repository interfaces.
Cumulative Cleanup (What becomes obsolete): The entire services/ directory is now functionally replaced by a combination of the new domain services and the upcoming application services.
Phase 5: Application Services (Orchestration)
New Directories to Create: internal/application/services/
Actions: This is pure creation. Write the new application services (content.go, auth.go, analytics.go, etc.) that orchestrate the repositories and domain services. The logic for these services is extracted from the old, "fat" handlers in the api/ directory.
Cumulative Cleanup (What becomes obsolete): A significant portion of the business logic inside the files in the api/ directory. The handler functions themselves still exist to serve HTTP requests, but their complex internal logic is now replaced by simple calls to these new application services.
Phase 6 & 7: Template System & HTTP Handlers
New Directories to Create: internal/presentation/{templates,http}
Actions:
Copy the entire html/ directory into internal/presentation/templates/, making minor DI-related changes to the rendering engine.
Create the new, "thin" HTTP handlers in internal/presentation/http/handlers/. These handlers will be very simple: parse the request, call a single method on an application service, and format the response.
Create the DTOs in internal/presentation/http/dto/.
Cumulative Cleanup (What becomes obsolete): The old html/ directory and the entire api/ directory are now fully replaced.
Phase 8 - 11: Final Infrastructure & Shared Code
New Directories to Create: internal/infrastructure/messaging/, internal/infrastructure/external/email/, pkg/
Actions: These phases are mostly moving and refactoring existing isolated code into the new structure. The events/, email/, utils/, and config/ directories are moved into their new homes under internal/ or pkg/.
Cumulative Cleanup (What becomes obsolete): The events/, email/, utils/, and config/ directories are fully superseded.
Phase 12: Application Entry Point & Final Cleanup
Directory to Create: cmd/server/
Actions:
Create a new main.go inside cmd/server/ that performs all dependency injection and bootstraps the new application using only code from the internal/ and pkg/ directories.
Modify the root go.mod file to reference this new main package.
Once the new main.go compiles and runs successfully, perform the final cleanup.
Final Cleanup: At this stage, you will delete all the old, now-obsolete directories from the root of the project:
rm -rf api/
rm -rf cache/
rm -rf events/
rm -rf html/
rm -rf models/
rm -rf services/
rm -rf tenant/
rm -rf utils/
rm -rf config/
rm main.go
After this step, the proper branch will contain only the new, clean architecture.
Phase 13: Final Review and Merge
Actions:
Run go mod tidy to clean up dependencies.
Thoroughly test the application on the proper branch.
Open a Pull Request to merge proper into main.
This approach ensures a safe, methodical, and verifiable transition from the old codebase to the new one, all within the proper branch
