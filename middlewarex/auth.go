package middlewarex

import "context"

// RequireAuth rejects requests without identity in context.
func RequireAuth[Req, Resp any]() Middleware[Req, Resp] {
	return func(next Handler[Req, Resp]) Handler[Req, Resp] {
		return func(ctx context.Context, req Req) (Resp, error) {
			if _, ok := IdentityFromContext(ctx); !ok {
				return *new(Resp), Unauthorized(errIdentityMissing)
			}

			return next(ctx, req)
		}
	}
}

// RequireRoles requires at least one role intersection.
func RequireRoles[Req, Resp any](roles ...string) Middleware[Req, Resp] {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		if role == "" {
			continue
		}
		allowed[role] = struct{}{}
	}

	return func(next Handler[Req, Resp]) Handler[Req, Resp] {
		return func(ctx context.Context, req Req) (Resp, error) {
			identity, ok := IdentityFromContext(ctx)
			if !ok {
				return *new(Resp), Unauthorized(errIdentityMissing)
			}

			for _, role := range identity.Roles {
				if _, ok := allowed[role]; ok {
					return next(ctx, req)
				}
			}

			return *new(Resp), Forbidden(errRolesMissing)
		}
	}
}

// RequireScopes requires all scopes to be present.
func RequireScopes[Req, Resp any](scopes ...string) Middleware[Req, Resp] {
	required := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		required[scope] = struct{}{}
	}

	return func(next Handler[Req, Resp]) Handler[Req, Resp] {
		return func(ctx context.Context, req Req) (Resp, error) {
			identity, ok := IdentityFromContext(ctx)
			if !ok {
				return *new(Resp), Unauthorized(errIdentityMissing)
			}

			granted := make(map[string]struct{}, len(identity.Scopes))
			for _, scope := range identity.Scopes {
				granted[scope] = struct{}{}
			}
			for scope := range required {
				if _, ok := granted[scope]; !ok {
					return *new(Resp), Forbidden(errScopesMissing)
				}
			}

			return next(ctx, req)
		}
	}
}
