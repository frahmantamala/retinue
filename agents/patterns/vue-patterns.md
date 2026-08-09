# Vue / Nuxt Patterns

Stack-specific code patterns for Vue 3 + Nuxt 4 projects.

---

## State Management with Pinia

```typescript
// stores/user.ts
import { defineStore } from 'pinia'

export const useUserStore = defineStore('user', () => {
  const users = ref<User[]>([])
  const loading = ref(false)

  // Computed for derived state (memoized)
  const activeUsers = computed(() =>
    users.value.filter(u => u.status === 'active')
  )

  // Actions for mutations
  async function fetchUsers() {
    loading.value = true
    try {
      const api = useUserApi()
      users.value = await api.getUsers()
    } catch (e) {
      error.value = e as Error
    } finally {
      loading.value = false
    }
  }

  return { users, loading, activeUsers, fetchUsers }
})
```

---

## Composables Pattern

```typescript
// composables/useUser.ts
import { useUserStore } from '~/stores/user'

export function useUser() {
  const userStore = useUserStore()
  const { users, loading } = storeToRefs(userStore)

  // Fetch with caching
  async function fetchUsers(options?: { force?: boolean }) {
    if (users.value.length > 0 && !options?.force) {
      return // Already cached
    }
    await userStore.fetchUsers()
  }

  return {
    users,
    loading,
    fetchUsers,
  }
}

// Usage in component
// <script setup>
// const { users, loading, fetchUsers } = useUser()
// onMounted(() => { fetchUsers() })
// </script>
```

---

## Optimistic Update Composable

Go optimistic when success is near-certain (>95%), the user needs instant feedback, and rollback is
cheap — likes, toggles, reordering. Stay pessimistic when the operation can genuinely fail or rollback
is complex: deletes, payments, form submissions.

```typescript
// composables/useOptimisticUpdate.ts
export function useOptimisticUpdate() {
  async function updateWithOptimism<T>(
    optimisticUpdate: () => void,
    apiCall: () => Promise<T>,
    rollback: () => void
  ) {
    // 1. Immediate UI update
    optimisticUpdate()

    try {
      // 2. API call in background
      await apiCall()
    } catch (error) {
      // 3. Rollback on failure
      rollback()
      console.error('Update failed, rolled back:', error)
    }
  }

  return { updateWithOptimism }
}
```

---

## Code Splitting & Lazy Loading

```vue
<script setup>
// Lazy load heavy components
const ChartComponent = defineAsyncComponent(() =>
  import('~/components/HeavyChart.vue')
)
</script>

<template>
  <ClientOnly>
    <Suspense>
      <ChartComponent v-if="showChart" />
      <template #fallback>
        <LoadingSpinner />
      </template>
    </Suspense>
  </ClientOnly>
</template>
```

---

## Image Optimization (NuxtImg)

```vue
<template>
  <NuxtImg
    src="/images/hero.jpg"
    alt="Hero image"
    width="1200"
    height="600"
    loading="lazy"
    format="webp"
    quality="80"
    sizes="sm:100vw md:50vw lg:400px"
  />
</template>
```

---

## Nuxt Route Rules (SSR/CSR)

SSR when the page is public and discovery matters — landing, blog/content, product pages (SEO, social
sharing, fast FCP). CSR when the page is private and interactive — dashboards, admin panels,
real-time views (no SEO value, complex client state).

```typescript
// nuxt.config.ts
export default defineNuxtConfig({
  routeRules: {
    // SSR for public pages
    '/': { ssr: true },
    '/blog/**': { ssr: true },
    '/products/**': { ssr: true },

    // CSR for private/dynamic pages
    '/dashboard/**': { ssr: false },
    '/admin/**': { ssr: false },
  }
})
```

---

## Vue Performance Tips

- **`v-once`** - Render once, skip future updates (static content)
- **`v-memo`** - Skip re-render unless specified deps change (expensive lists)
- **`key` on `v-for`** - Always use unique, stable keys (not array index)
- **`shallowRef`** - Use for large objects where you replace the whole value
- **`computed`** - Always use for derived state (auto-memoized)

```vue
<template>
  <!-- Static content: render once -->
  <footer v-once>
    <p>{{ companyInfo }}</p>
  </footer>

  <!-- Expensive list: only re-render changed items -->
  <div v-for="item in list" :key="item.id" v-memo="[item.id, item.updated]">
    <ExpensiveComponent :item="item" />
  </div>
</template>
```

---

## Separation of Concerns

| Layer | Contains | Doesn't contain |
|------|----------|-----------------|
| **UI components** (`ui/`) | Props, events, styling, rendering | API calls, business logic |
| **Feature components** (`features/`) | Composition, layout, event handling | Direct API calls — go through the logic layer |
| **Logic layer** (composables/services) | Business logic, API calls, data fetching | UI rendering |
| **State** (stores) | Global state, actions, derived state | UI logic, direct API calls |
| **Utils** | Pure functions | State, API calls |

Extract a child component when the logic is reused, needs its own tests, or the parent has outgrown
roughly 100 lines. Keep it inline when it is used once and extracting would be abstraction for its
own sake.

---

## File Structure Convention

```
components/
├── ui/                      # Pure UI components (no logic)
│   ├── Button/
│   ├── Input/
│   └── Card/
├── features/                # Feature-specific components
│   └── user/
│       ├── UserProfile.vue
│       └── useUser.ts       # Co-located composable
composables/                 # Reusable logic (separated from UI)
├── useAuth.ts
├── useApi.ts
└── useOptimisticUpdate.ts
stores/                      # Pinia stores
├── user.ts
├── auth.ts
└── ui.ts
utils/                       # Pure utility functions
├── validation.ts
└── formatting.ts
```
