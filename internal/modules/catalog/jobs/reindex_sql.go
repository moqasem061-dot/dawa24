package jobs

// SQL for the catalog.product_index read model.
//
// Two things were wrong here and are fixed in this file.
//
// First, `variant_id` was the literal NULL and `stock_quantity` was the literal
// 0, for every row. The columns existed, the reindex wrote constants into them,
// and nothing complained. On the live database that produced 28,786 indexed
// products of which **zero** had a variant and **zero** had stock, while
// inventory.stocks held 14,539 real rows. Any query of the documented form
// "active products with stock_quantity > 0" therefore returned nothing at all.
//
// Second, the index only ever held parent rows. A vendor's sellable offer is a
// *variant*, so an index without variant rows cannot answer "who sells this",
// which is the question the whole marketplace is built around. The
// `product_type` column was already there, and already said 'parent', so the
// intent was clearly to carry both.
//
// Stock is summed across warehouses for a variant. A variant stocked in three
// warehouses is one offer with the combined quantity; the per-warehouse split
// matters to fulfilment, not to whether a buyer can order it.

// stockJoin aggregates inventory per variant. Kept separate so the parent and
// variant queries cannot drift in how they count stock.
const stockJoin = `
	LEFT JOIN LATERAL (
		SELECT COALESCE(SUM(s.quantity), 0)::int AS qty
		FROM inventory.stocks s
		WHERE s.product_variant_id = v.id
	) st ON TRUE`

// parentSelect projects a catalogue product into an index row.
//
// A parent's stock is the sum of its variants' stock: the parent row answers
// "is this product obtainable at all", which is what catalogue search needs.
const parentSelect = `
	SELECT
		'p_' || p.id::text AS unique_row_id,
		p.id AS product_id,
		NULL::bigint AS variant_id,
		p.sku,
		p.name->>'ar' AS name_ar,
		p.name->>'en' AS name_en,
		CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, ''), COALESCE(p.sku, '')) AS search_text,
		CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_ar,
		CONCAT_WS(' ', COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_en,
		CONCAT_WS(' ', platform.normalize_arabic(COALESCE(p.name->>'ar', '')), COALESCE(p.name->>'en', ''), COALESCE(p.sku, '')) AS search_simple,
		o.legal_name AS organization_name,
		b.city_id AS branch_city,
		p.scientific_name,
		p.price,
		p.discount,
		COALESCE(pstock.qty, 0) AS stock_quantity,
		p.category_id,
		p.brand_id,
		(p.discount > 0) AS has_discount,
		p.discount AS discount_percentage,
		ROUND(p.price * (1 - p.discount / 100), 2) AS price_after_discount,
		p.organization_id,
		p.branch_id,
		p.status::text AS status,
		'parent' AS product_type,
		COALESCE(p.institutional_work_ids, '{}'::bigint[]) AS institutional_work_ids
	FROM catalog.products p
	JOIN org.organizations o ON p.organization_id = o.id
	LEFT JOIN org.branches b ON p.branch_id = b.id
	LEFT JOIN LATERAL (
		SELECT COALESCE(SUM(s.quantity), 0)::int AS qty
		FROM catalog.product_variants pv
		JOIN inventory.stocks s ON s.product_variant_id = pv.id
		WHERE pv.product_id = p.id AND pv.deleted_at IS NULL
	) pstock ON TRUE`

// variantSelect projects a vendor's offer into an index row.
//
// The variant carries its own price, discount and SKU because that is the offer;
// it inherits the parent's searchable identity because that is what a buyer
// types. institutional_work_ids come from the parent: the restriction is a
// property of the product, not of one vendor's packaging of it.
const variantSelect = `
	SELECT
		'v_' || v.id::text AS unique_row_id,
		v.product_id AS product_id,
		v.id AS variant_id,
		COALESCE(NULLIF(v.sku, ''), p.sku) AS sku,
		COALESCE(v.name->>'ar', p.name->>'ar') AS name_ar,
		COALESCE(v.name->>'en', p.name->>'en') AS name_en,
		CONCAT_WS(' ', COALESCE(v.name->>'ar', ''), COALESCE(v.name->>'en', ''), COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, ''), COALESCE(v.sku, '')) AS search_text,
		CONCAT_WS(' ', COALESCE(v.name->>'ar', ''), COALESCE(p.name->>'ar', ''), COALESCE(p.scientific_name, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_ar,
		CONCAT_WS(' ', COALESCE(v.name->>'en', ''), COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.legal_name, '')) AS search_en,
		CONCAT_WS(' ', platform.normalize_arabic(COALESCE(v.name->>'ar', p.name->>'ar', '')), COALESCE(v.name->>'en', p.name->>'en', ''), COALESCE(v.sku, '')) AS search_simple,
		o.legal_name AS organization_name,
		b.city_id AS branch_city,
		p.scientific_name,
		v.price,
		v.discount,
		COALESCE(st.qty, 0) AS stock_quantity,
		p.category_id,
		p.brand_id,
		(v.discount > 0) AS has_discount,
		v.discount AS discount_percentage,
		ROUND(v.price * (1 - v.discount / 100), 2) AS price_after_discount,
		v.organization_id,
		p.branch_id,
		v.status::text AS status,
		'variant' AS product_type,
		COALESCE(p.institutional_work_ids, '{}'::bigint[]) AS institutional_work_ids
	FROM catalog.product_variants v
	JOIN catalog.products p ON v.product_id = p.id
	JOIN org.organizations o ON v.organization_id = o.id
	LEFT JOIN org.branches b ON p.branch_id = b.id` + stockJoin

// indexColumns is the shared INSERT column list.
const indexColumns = `
	unique_row_id, product_id, variant_id, sku, name_ar, name_en,
	search_text, search_ar, search_en, search_simple,
	organization_name, branch_city, scientific_name, price, discount,
	stock_quantity, category_id, brand_id, has_discount,
	discount_percentage, price_after_discount, organization_id, branch_id,
	status, product_type, institutional_work_ids`

// upsertAssignments is the shared ON CONFLICT body.
const upsertAssignments = `
	product_id = EXCLUDED.product_id,
	variant_id = EXCLUDED.variant_id,
	sku = EXCLUDED.sku,
	name_ar = EXCLUDED.name_ar,
	name_en = EXCLUDED.name_en,
	search_text = EXCLUDED.search_text,
	search_ar = EXCLUDED.search_ar,
	search_en = EXCLUDED.search_en,
	search_simple = EXCLUDED.search_simple,
	organization_name = EXCLUDED.organization_name,
	branch_city = EXCLUDED.branch_city,
	scientific_name = EXCLUDED.scientific_name,
	price = EXCLUDED.price,
	discount = EXCLUDED.discount,
	stock_quantity = EXCLUDED.stock_quantity,
	category_id = EXCLUDED.category_id,
	brand_id = EXCLUDED.brand_id,
	has_discount = EXCLUDED.has_discount,
	discount_percentage = EXCLUDED.discount_percentage,
	price_after_discount = EXCLUDED.price_after_discount,
	organization_id = EXCLUDED.organization_id,
	branch_id = EXCLUDED.branch_id,
	status = EXCLUDED.status,
	product_type = EXCLUDED.product_type,
	institutional_work_ids = EXCLUDED.institutional_work_ids,
	updated_at = now()`

const (
	activeParents  = ` WHERE p.deleted_at IS NULL AND o.deleted_at IS NULL AND p.status = 'active'`
	activeVariants = ` WHERE v.deleted_at IS NULL AND p.deleted_at IS NULL AND o.deleted_at IS NULL AND v.status = 'active'`
)

// Full-sweep statements.
var (
	sweepParents  = `INSERT INTO catalog.product_index (` + indexColumns + `)` + parentSelect + activeParents + `;`
	sweepVariants = `INSERT INTO catalog.product_index (` + indexColumns + `)` + variantSelect + activeVariants + `;`
)

// Single-product statements. The variant upsert refreshes every variant of the
// product, because a price change on the parent and a stock movement on one
// variant both arrive as "reindex product N".
var (
	upsertParent = `INSERT INTO catalog.product_index (` + indexColumns + `)` + parentSelect +
		` WHERE p.id = $1 AND p.deleted_at IS NULL AND o.deleted_at IS NULL
		  ON CONFLICT (unique_row_id) DO UPDATE SET ` + upsertAssignments + `;`

	upsertVariants = `INSERT INTO catalog.product_index (` + indexColumns + `)` + variantSelect +
		` WHERE v.product_id = $1 AND v.deleted_at IS NULL AND p.deleted_at IS NULL AND o.deleted_at IS NULL
		  ON CONFLICT (unique_row_id) DO UPDATE SET ` + upsertAssignments + `;`

	// Variants that are no longer active must leave the index, or a deactivated
	// offer keeps appearing as available.
	pruneVariants = `
		DELETE FROM catalog.product_index
		WHERE product_type = 'variant'
		  AND product_id = $1
		  AND variant_id NOT IN (
		      SELECT v.id FROM catalog.product_variants v
		      WHERE v.product_id = $1 AND v.deleted_at IS NULL AND v.status = 'active'
		  );`
)
