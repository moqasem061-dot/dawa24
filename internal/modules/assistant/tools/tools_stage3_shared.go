package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

type stage3Spec struct {
	kind        assistant.ProjectionKind
	key         string
	description string
	scopes      []rbac.Scope
	permissions []string
	handleKind  handles.Kind
	handleField string
	detail      bool
}

type stage3Args struct {
	dateRangeArgs
	Search       string `json:"search,omitempty"`
	Status       string `json:"status,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Offset       int    `json:"offset,omitempty"`
	Handle       string `json:"handle,omitempty"`
	Offer        string `json:"offer,omitempty"`
	Invoice      string `json:"invoice,omitempty"`
	Run          string `json:"run,omitempty"`
	Variant      string `json:"variant,omitempty"`
	Warehouse    string `json:"warehouse,omitempty"`
	Transfer     string `json:"transfer,omitempty"`
	Request      string `json:"request,omitempty"`
	Organization string `json:"organization,omitempty"`
	Member       string `json:"member,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Product      string `json:"product,omitempty"`
	Order        string `json:"order,omitempty"`
	Shipment     string `json:"shipment,omitempty"`
}

func projectionListSchema(extra map[string]any) map[string]any {
	return objectSchema(pageProps(dateProps(extra)))
}

func projectionDetailSchema(field string) map[string]any {
	return objectSchema(map[string]any{
		field: strProp("Opaque reference returned by the related assistant list."),
	}, field)
}

func projectionTool(r *Registry, spec stage3Spec, params map[string]any) Tool {
	return Tool{
		Name: spec.key, Description: spec.description, Params: params,
		Scopes: spec.scopes, Permissions: spec.permissions,
		Handler: r.stage3Handler(spec),
	}
}

func (r *Registry) stage3Handler(spec stage3Spec) Handler {
	return func(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
		if r.projections == nil {
			return Result{Note: "This assistant data source is unavailable."}, nil
		}
		var args stage3Args
		if err := decode(raw, &args); err != nil {
			return Result{}, err
		}
		rng, err := args.parse(90)
		if err != nil {
			return Result{}, err
		}
		search, err := trimSearch(args.Search)
		if err != nil {
			return Result{}, err
		}
		offset, err := clampOffset(args.Offset)
		if err != nil {
			return Result{}, err
		}
		q := assistant.ProjectionQuery{
			Kind: spec.kind, Range: rng, Search: search, Status: strings.TrimSpace(args.Status),
			Limit: clampLimit(args.Limit), Offset: offset,
		}
		if spec.detail {
			token := stage3Handle(args, spec.handleField)
			id, resolveErr := r.resolveHandle(actor, spec.handleKind, token)
			if resolveErr != nil {
				return Result{}, resolveErr
			}
			q.ID = id
		}
		if args.Branch != "" {
			q.BranchID, err = r.resolveHandle(actor, handles.KindBranch, args.Branch)
			if err != nil {
				return Result{}, err
			}
		}
		if args.Product != "" {
			q.ProductID, err = r.resolveHandle(actor, handles.KindProduct, args.Product)
			if err != nil {
				return Result{}, err
			}
		}
		pageResult, err := r.projections.ReadProjection(ctx, actor, q)
		if err != nil {
			return Result{}, err
		}
		for i := range pageResult.Rows {
			row := &pageResult.Rows[i]
			if row.ID <= 0 || spec.handleKind == "" {
				continue
			}
			row.Handle = r.issue(actor, spec.handleKind, row.ID)
			if spec.handleField != "" {
				if row.Values == nil {
					row.Values = map[string]any{}
				}
				if existing, ok := row.Values[spec.handleField]; ok && existing != nil && fmt.Sprint(existing) != "" {
					row.Values[spec.handleField+"_ref"] = row.Handle
				} else {
					row.Values[spec.handleField] = row.Handle
					row.Handle = ""
				}
			}
		}
		if len(pageResult.Rows) == 0 {
			return Result{Note: "No matching records were found."}, nil
		}
		return page(pageResult, spec.key), nil
	}
}

func stage3Handle(args stage3Args, field string) string {
	var val string
	switch field {
	case "offer":
		val = args.Offer
	case "invoice":
		val = args.Invoice
	case "run":
		val = args.Run
	case "variant":
		val = args.Variant
	case "warehouse":
		val = args.Warehouse
	case "transfer":
		val = args.Transfer
	case "request":
		val = args.Request
	case "organization":
		val = args.Organization
	case "member":
		val = args.Member
	case "order":
		val = args.Order
	case "shipment":
		val = args.Shipment
	}
	if val != "" {
		return val
	}
	return args.Handle
}
