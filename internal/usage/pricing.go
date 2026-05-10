package usage

import (
	"log/slog"
	"sync"
)

type modelPrice struct {
	inputPerToken  float64
	outputPerToken float64
}

var prices = map[string]modelPrice{
	"gpt-5.5":      {5.00 / 1e6, 30.00 / 1e6},
	"gpt-5.4":      {2.50 / 1e6, 15.00 / 1e6},
	"gpt-5.4-mini": {0.75 / 1e6, 4.50 / 1e6},
	"gpt-5.4-nano": {0.20 / 1e6, 1.25 / 1e6},
	"gpt-5":        {1.25 / 1e6, 10.00 / 1e6},
	"gpt-5-mini":   {0.25 / 1e6, 2.00 / 1e6},
	"o3":           {2.00 / 1e6, 8.00 / 1e6},
	"o3-mini":      {1.10 / 1e6, 4.40 / 1e6},
	"gpt-4o":       {2.50 / 1e6, 10.00 / 1e6},
	"gpt-4o-mini":  {0.15 / 1e6, 0.60 / 1e6},
}

var warnedModels sync.Map

func CostUSD(model string, promptTokens, completionTokens int) float64 {
	p, ok := prices[model]
	if !ok {
		if _, loaded := warnedModels.LoadOrStore(model, struct{}{}); !loaded {
			slog.Warn("unknown model for pricing", "model", model)
		}
		return 0.0
	}
	return float64(promptTokens)*p.inputPerToken + float64(completionTokens)*p.outputPerToken
}
