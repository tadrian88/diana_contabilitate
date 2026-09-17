package requestactor

import "context"

type Actor struct {
	ID                  string
	Display             string
	Persona             string
	AuthorizedClientIDs []string
	AllClients          bool
}

type contextKey struct{}

func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, contextKey{}, actor)
}

func FromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(contextKey{}).(Actor)
	return actor, ok
}

// AllowsClient is the existing actor/grant boundary, not a production authentication provider.
func (a Actor) AllowsClient(id string) bool {
	if a.ID == "" {
		return false
	}
	if a.AllClients {
		return true
	}
	for _, v := range a.AuthorizedClientIDs {
		if v == id {
			return true
		}
	}
	return false
}
