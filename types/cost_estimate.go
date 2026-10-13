package types

import "math"

// PromptTokenAccounting specifies whether Usage.PromptTokens includes cache tokens.
// Providers differ: select explicitly for the usage being priced.
type PromptTokenAccounting uint8

const (
	// PromptTokensInclusiveCache subtracts cache reads/writes from PromptTokens.
	PromptTokensInclusiveCache PromptTokenAccounting = iota + 1
	// PromptTokensUncached treats PromptTokens as uncached input and adds caches.
	PromptTokensUncached
)

// UsageCostOptions supplies cache rates per 1,000 tokens in ModelCost's currency.
// Nil means unavailable pricing; a pointer to zero means a known free category.
type UsageCostOptions struct {
	PromptAccounting PromptTokenAccounting
	CacheReadRate    *float64
	CacheWriteRate   *float64
}

// UsageCostEstimate contains known category charges, and a Total only when Known.
// Nil charges have unavailable pricing; nonnil zero charges are known zero.
// This is an estimate from supplied rates and usage, not a provider invoice.
type UsageCostEstimate struct {
	Currency   string
	Known      bool
	Input      *float64
	Output     *float64
	CacheRead  *float64
	CacheWrite *float64
	Total      *float64
}

// EstimateUsageCost prices usage without assuming provider cache accounting or
// cache discounts. CompletionTokens already includes reasoning when reported by
// providers; ReasoningTokens and TotalTokens are not additional billing categories.
func EstimateUsageCost(cost *ModelCost, usage Usage, options UsageCostOptions) (UsageCostEstimate, error) {
	input, err := uncachedPromptTokens(usage, options.PromptAccounting)
	if err != nil {
		return UsageCostEstimate{}, err
	}
	if err := validateUsageRates(cost, options); err != nil {
		return UsageCostEstimate{}, err
	}
	result := UsageCostEstimate{}
	if cost != nil {
		result.Currency = cost.Currency
		result.Input = tokenCharge(input, &cost.InputTokens)
		result.Output = tokenCharge(usage.CompletionTokens, &cost.OutputTokens)
	}
	result.CacheRead = tokenCharge(usage.CacheReadTokens, options.CacheReadRate)
	result.CacheWrite = tokenCharge(usage.CacheWriteTokens, options.CacheWriteRate)
	charges := []*float64{result.Input, result.Output, result.CacheRead, result.CacheWrite}
	total, known := 0.0, true
	for _, charge := range charges {
		if charge == nil {
			known = false
			continue
		}
		if math.IsInf(*charge, 0) || math.IsNaN(*charge) {
			return UsageCostEstimate{}, invalidCost("nonfinite token charge")
		}
		total += *charge
	}
	if math.IsInf(total, 0) || math.IsNaN(total) {
		return UsageCostEstimate{}, invalidCost("nonfinite total charge")
	}
	if known {
		result.Known = true
		result.Total = &total
	}
	return result, nil
}

func uncachedPromptTokens(usage Usage, accounting PromptTokenAccounting) (int, error) {
	if usage.PromptTokens < 0 || usage.CompletionTokens < 0 || usage.TotalTokens < 0 ||
		usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 {
		return 0, invalidCost("negative token count")
	}
	switch accounting {
	case PromptTokensUncached:
		return usage.PromptTokens, nil
	case PromptTokensInclusiveCache:
		// Sequential subtraction avoids overflow when two huge cache counts are supplied.
		remaining := usage.PromptTokens - usage.CacheReadTokens
		if remaining < 0 || usage.CacheWriteTokens > remaining {
			return 0, invalidCost("cache counts exceed prompt tokens")
		}
		return remaining - usage.CacheWriteTokens, nil
	default:
		return 0, invalidCost("explicit prompt token accounting is required")
	}
}

func validateUsageRates(cost *ModelCost, options UsageCostOptions) error {
	rates := []*float64{options.CacheReadRate, options.CacheWriteRate}
	if cost != nil {
		rates = append(rates, &cost.InputTokens, &cost.OutputTokens)
	}
	for _, rate := range rates {
		if rate != nil && (*rate < 0 || math.IsNaN(*rate) || math.IsInf(*rate, 0)) {
			return invalidCost("invalid token rate")
		}
	}
	return nil
}

func tokenCharge(tokens int, rate *float64) *float64 {
	if tokens == 0 {
		zero := 0.0
		return &zero
	}
	if rate == nil {
		return nil
	}
	amount := float64(tokens) / 1000 * *rate
	return &amount
}

func invalidCost(detail string) error { return ErrInvalidRequest.WithDetails(detail) }

// EstimateUsageCost looks up a model's base pricing and prices all usage categories.
// An existing model without Cost returns an unknown estimate; a missing model errors.
func (r *ModelRegistry) EstimateUsageCost(modelID string, usage Usage, options UsageCostOptions) (UsageCostEstimate, error) {
	model, ok := r.Get(modelID)
	if !ok {
		return UsageCostEstimate{}, ErrModelNotFound.WithModel(modelID)
	}
	return EstimateUsageCost(model.Cost, usage, options)
}

// EstimateModelUsageCost prices usage using DefaultModelRegistry.
func EstimateModelUsageCost(modelID string, usage Usage, options UsageCostOptions) (UsageCostEstimate, error) {
	return DefaultModelRegistry.EstimateUsageCost(modelID, usage, options)
}
