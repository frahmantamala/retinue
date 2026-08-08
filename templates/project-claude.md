# Project: [Project Name]

## Tech Stack

<!-- Fill in your project's stack -->

**Backend:**
- Language/Framework: [e.g. Go, Node/Express, Node/NestJS, Python/FastAPI]
- Database: [e.g. PostgreSQL, MySQL, MongoDB]
- ORM/Query: [e.g. sqlx, Prisma, TypeORM, raw SQL]

**Frontend:**
- Framework: [e.g. Vue 3/Nuxt 4, React/Next.js, Angular, Svelte/SvelteKit]
- State Management: [e.g. Pinia, Redux/Zustand, NgRx, Svelte stores]
- Styling: [e.g. Tailwind CSS, CSS Modules, styled-components]

**Infrastructure:**
- [e.g. Docker, Kubernetes, AWS, Vercel]

---

## Architecture

<!-- Describe your project's architecture pattern -->
<!-- e.g. modular monolith, microservices, monorepo, etc. -->

### File Structure

```
<!-- Fill in your project's directory structure -->
<!-- Example for a Go modular monolith: -->
<!--
api/
├── openapi3.yml
cmd/
├── cmd.go
├── http_server.go
├── migrate.go
db/
├── migrations/
internal/
├── config.go
├── core/
│   └── common/
│       └── datamodel/
├── {feature}/
│   ├── dto/
│   ├── service.go
│   ├── {feature}.go
│   ├── postgresql/
│   ├── handler.go
│   └── endpoint.go
└── transport/
-->

<!-- Example for a Next.js app: -->
<!--
src/
├── app/
│   ├── layout.tsx
│   ├── page.tsx
│   └── [feature]/
├── components/
│   ├── ui/
│   └── features/
├── hooks/
├── stores/
├── lib/
└── types/
-->
```

### Layer Responsibilities

<!-- Define what each layer does and doesn't do -->
<!-- Example: -->
<!--
| Layer | DO | DON'T |
|-------|-----|-------|
| Models/Entities | Data structures, shared types | Business logic |
| DTOs | API contracts, validation | DB queries |
| Services | Orchestrate use cases | Direct DB access |
| Domain | Business rules, validations | HTTP concerns |
| Repository | Data access, queries | Business logic |
| Handlers | HTTP translation, input parsing | Business logic |
-->

---

## Conventions

### Naming
<!-- e.g. camelCase for functions, PascalCase for components, snake_case for DB columns -->

### Patterns
<!-- List any patterns specific to this project -->
<!-- e.g. "Load go patterns" for Go concurrency/idempotency -->
<!-- e.g. "Load vue patterns" for Vue/Nuxt component patterns -->

### Testing
<!-- e.g. test files colocated, __tests__ directory, naming convention -->

---

## Commands

<!-- Common commands for this project -->
```
# Development
# [e.g. go run ./cmd, npm run dev, pnpm dev]

# Testing
# [e.g. go test ./..., npm test, pnpm test]

# Build
# [e.g. go build, npm run build]

# Migrations
# [e.g. goose up, npx prisma migrate dev]

# Linting
# [e.g. golangci-lint run, npm run lint]
```
