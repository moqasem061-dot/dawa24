package postgres

// The review upsert statements (kept out of the loop for readability).
const (
	revUpsertSQL = `UPDATE erp.review_items SET payload = $4::jsonb, batch_id = NULLIF($5,0), candidates = $6::jsonb, updated_at = now()
		WHERE organization_id = $1 AND integration_id = $2 AND external_code = $3 AND state = 'open'`
	revInsertSQL = `INSERT INTO erp.review_items (organization_id, integration_id, external_code, payload, reason_code, batch_id, candidates)
		VALUES ($1, $2, $3, $4::jsonb, $5, NULLIF($6,0), $7::jsonb)`
)
