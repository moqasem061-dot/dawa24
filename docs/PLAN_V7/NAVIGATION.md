# Navigation audit (PLAN_V7 Task 5.5)

Rule: every **top-level** admin page appears in the sidebar or is deleted.
Detail and sub-views (`/{id}`, `/{id}/edit`, and a screen's own tabs) are
reached from their parent list, not from the sidebar.

## Result

| | Before | After |
|---|---:|---:|
| Real admin page routes (non-redirect GET) | 80 | 80 |
| Sidebar entries | 23 | 67 |
| Top-level pages unreachable from navigation | 59 | **0** |
| Dead template targets | 1 (`/ready`) | 1 (`/ready`) |

`/ready` is a health endpoint registered outside the UI mux; it is excluded by
design, not an orphan.

## Admin sidebar — 9 groups

Group headings and order follow
`Laravel/resources/views/components/layouts/admin.blade.php`.

| Group | Entries |
|---|---|
| الرئيسية والمتابعة | dashboard, audit, notifications, activity logs, error logs, admin notifications |
| المؤسسة والمشرفين | organizations, branches, users, admins, roles, documents, approvals, admin roles, permissions catalogue, employee activity, user addresses, org membership, deletion requests, document requests, chat tree, chat history |
| الإعدادات والمحتوى | settings, institutional works, cities, policies, countries, social channels, highlight sections, page content, integrations, medicine finder, services |
| المنتجات والمخزون | products, categories, catalog import, brands, vendor variants, promoted products, integration products, saving products, stock, warehouses, temp warehouses, weekly coverage |
| المبيعات والمالية | orders, plans, offer orders, invoices, payments, wallets, order earnings, offer earnings, active subscriptions |
| العروض والتسويق | offers, offer packages, offer locations, ads, ad plans, analytics |
| الأدوات والخدمات | messages, jobs, session plans, user reports, system resources |
| إدارة المحذوفات | trash list |
| المطور والتشخيص | developer tools |

Every entry is wrapped in `canAccessAdmin(ctx, "<permission>")`, so a staff
member never sees a link that 404s for them.

## Sub-views reached from their parent (correctly not in the sidebar)

| Sub-view | Parent |
|---|---|
| `/admin/offers-packages/{packages,sponsorships,promotions,views,clicks}` | `/admin/offers-packages` hub |
| `/admin/plans-info`, `/admin/plan-types`, `/admin/plan-features`, `/admin/plans/subscriptions` | `/admin/plans` sub-navigation |
| `/admin/session-plan/requests` | `/admin/session-plan` |
| `/admin/weekly-coverages/add` | the coverage list |
| `/admin/products-saving/import` | saving products |
| `/admin/{my,admins,import,plan,user-plan}/temparte-warehouses` | the temp-warehouses screen |
| `/admin/first-look` | reached by the first-login middleware, never linked |

Two of these were genuinely unreachable and were fixed rather than documented
away: the offers-packages hub linked the `/admin/offer-sponsorships` **alias**
instead of the canonical path, and the plans screen had no link to its three
sub-views at all.

## Vendor sidebar — 6 groups

المنشأة والفروع · الكتالوج والمخزون · العروض والتسويق · الطلبات والمالية ·
الأدوات والتحليلات · المحتوى والسياسات

Previously 32 flat entries with no grouping.

## Retired paths (301, not deleted)

They were linked from sidebars and may be bookmarked.

| From | To |
|---|---|
| `/settings/{profile,addresses,security,organization,preferences,payment-methods}` | `/settings?tab=…` |
| `/admin/policies` | `/admin/settings?tab=policies` |
| `/admin/deletes-lists*` | `/admin/trash-list*` |
| `/admin/{vendors,suppliers}` | `/admin/organizations?type=vendor` |
| `/admin/offer-sponsorships` | `/admin/offers-packages/sponsorships` |
| `/admin/saveing-products` | `/admin/saving-products` |
| `/admin/{full-user,customer-list,vendor-list,admin-list,admins}` | `/admin/users?type=…` |
| `/customer/cpanel` | `/customer/dashboard` |
