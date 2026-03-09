package llm

type ModelTier string

const (
	TierLocal   ModelTier = "local"
	TierPremium ModelTier = "premium"
	TierCode    ModelTier = "code"
)

type Router struct{}

func NewRouter() *Router {
	return &Router{}
}

func (r *Router) Route(taskType string, priority int) ModelTier {
	switch {
	case taskType == "classification":
		return TierLocal
	case taskType == "code_generation":
		return TierCode
	case priority < 50:
		return TierPremium
	default:
		return TierLocal
	}
}
