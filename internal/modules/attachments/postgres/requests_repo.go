package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func (r *Repository) SoftDelete(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE platform_admin.documents SET deleted_at = now(), updated_at = now() WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id)
		return err
	})
}

func (r *Repository) HardDelete(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM platform_admin.documents WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id)
		return err
	})
}

func (r *Repository) CreateDocumentRequest(ctx context.Context, req *attachments.DocumentRequest) (*attachments.DocumentRequest, error) {
	var out attachments.DocumentRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.document_requests (
				organization_id, requested_by, document_type, title, description, deadline_at, status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
			RETURNING id, organization_id, requested_by, document_type, title, description, deadline_at, status, submitted_doc_id, created_at, updated_at;
		`
		var st string
		if req.Status == "" {
			st = string(attachments.DocReqPending)
		} else {
			st = string(req.Status)
		}
		var dt string = string(req.DocumentType)
		var subDocID *int64
		err := tx.QueryRow(txCtx, query,
			req.OrganizationID, req.RequestedBy, dt, req.Title, req.Description, req.DeadlineAt, st,
		).Scan(
			&out.ID, &out.OrganizationID, &out.RequestedBy, &dt, &out.Title, &out.Description, &out.DeadlineAt, &st, &subDocID, &out.CreatedAt, &out.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("document_requests.Create: %w", err)
		}
		out.DocumentType = attachments.DocumentType(dt)
		out.Status = attachments.DocumentRequestStatus(st)
		out.SubmittedDocID = subDocID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *Repository) ListRequestsByOrg(ctx context.Context, orgID int64) ([]*attachments.DocumentRequest, error) {
	var list []*attachments.DocumentRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT dr.id, dr.organization_id, COALESCE(o.legal_name, ''), dr.requested_by, dr.document_type,
			       dr.title, dr.description, dr.deadline_at, dr.status, dr.submitted_doc_id, dr.created_at, dr.updated_at
			FROM platform_admin.document_requests dr
			LEFT JOIN org.organizations o ON o.id = dr.organization_id
			WHERE dr.organization_id = $1
			ORDER BY dr.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return fmt.Errorf("document_requests.ListByOrg: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item attachments.DocumentRequest
			var dt, st string
			var subDocID *int64
			if err := rows.Scan(
				&item.ID, &item.OrganizationID, &item.OrgName, &item.RequestedBy, &dt,
				&item.Title, &item.Description, &item.DeadlineAt, &st, &subDocID, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return fmt.Errorf("document_requests.Scan: %w", err)
			}
			item.DocumentType = attachments.DocumentType(dt)
			item.Status = attachments.DocumentRequestStatus(st)
			item.SubmittedDocID = subDocID
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ListAllRequests(ctx context.Context) ([]*attachments.DocumentRequest, error) {
	var list []*attachments.DocumentRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT dr.id, dr.organization_id, COALESCE(o.legal_name, ''), dr.requested_by, dr.document_type,
			       dr.title, dr.description, dr.deadline_at, dr.status, dr.submitted_doc_id, dr.created_at, dr.updated_at
			FROM platform_admin.document_requests dr
			LEFT JOIN org.organizations o ON o.id = dr.organization_id
			ORDER BY dr.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return fmt.Errorf("document_requests.ListAll: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item attachments.DocumentRequest
			var dt, st string
			var subDocID *int64
			if err := rows.Scan(
				&item.ID, &item.OrganizationID, &item.OrgName, &item.RequestedBy, &dt,
				&item.Title, &item.Description, &item.DeadlineAt, &st, &subDocID, &item.CreatedAt, &item.UpdatedAt,
			); err != nil {
				return fmt.Errorf("document_requests.Scan: %w", err)
			}
			item.DocumentType = attachments.DocumentType(dt)
			item.Status = attachments.DocumentRequestStatus(st)
			item.SubmittedDocID = subDocID
			list = append(list, &item)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) UpdateRequestStatus(ctx context.Context, id int64, status attachments.DocumentRequestStatus, submittedDocID *int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.document_requests
			SET status = $1, submitted_doc_id = COALESCE($2, submitted_doc_id), updated_at = now()
			WHERE id = $3;
		`
		tag, err := tx.Exec(txCtx, query, string(status), submittedDocID, id)
		if err != nil {
			return fmt.Errorf("document_requests.UpdateStatus: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("document_request")
		}
		return nil
	})
}

func (r *Repository) FulfillRequestByDoc(ctx context.Context, orgID int64, docType attachments.DocumentType, docID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE platform_admin.document_requests
			SET status = 'fulfilled', submitted_doc_id = $1, updated_at = now()
			WHERE organization_id = $2
			  AND (document_type = $3 OR document_type = '' OR document_type = 'other')
			  AND status IN ('pending', 'submitted');
		`
		_, err := tx.Exec(txCtx, query, docID, orgID, string(docType))
		return err
	})
}
