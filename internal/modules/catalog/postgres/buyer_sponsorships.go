package postgres

// Aggregate sponsorships once, before sorting the catalog, instead of running
// a correlated scan for each eligible variant. Product and variant targets
// remain distinct, and duplicate promotions retain their highest active tier.
const buyerSponsorshipCTE = `WITH active_sponsorships AS (
 SELECT COALESCE(sp.item_id, sp.offer_id) AS target_id,
        CASE WHEN sp.item_type = 'product' THEN 'product' ELSE 'variant' END AS target_type,
        MAX(op.tier_level) AS tier
 FROM promo.offer_sponsorships sp
 JOIN promo.offer_packages op ON op.id = sp.package_id
 WHERE sp.status = 'active' AND sp.admin_status = 'approved'
   AND (sp.starts_at IS NULL OR sp.starts_at <= now())
   AND (sp.expires_at IS NULL OR sp.expires_at >= now())
   AND sp.item_type IN ('offer', 'variant', 'product')
 GROUP BY 1, 2
) `

const buyerSponsorshipJoins = `
 LEFT JOIN active_sponsorships sv ON sv.target_type = 'variant' AND sv.target_id = v.id
 LEFT JOIN active_sponsorships sprod ON sprod.target_type = 'product' AND sprod.target_id = v.product_id`
