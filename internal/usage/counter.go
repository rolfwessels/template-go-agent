package usage

import (
	"fmt"
	"math"
	"sync/atomic"

	"github.com/dustin/go-humanize"
)

type Counter struct {
	promptTokens     atomic.Int64
	completionTokens atomic.Int64
	costBits         atomic.Uint64
}

func NewCounter() *Counter {
	return &Counter{}
}

func (c *Counter) add(u TokenUsage, cost float64) {
	c.promptTokens.Add(int64(u.PromptTokens))
	c.completionTokens.Add(int64(u.CompletionTokens))
	atomicAddFloat64(&c.costBits, cost)
}

func atomicAddFloat64(bits *atomic.Uint64, delta float64) {
	for {
		old := bits.Load()
		if bits.CompareAndSwap(old, math.Float64bits(math.Float64frombits(old)+delta)) {
			return
		}
	}
}

func (c *Counter) StatusLine() string {
	prompt := c.promptTokens.Load()
	completion := c.completionTokens.Load()
	cost := math.Float64frombits(c.costBits.Load())
	return fmt.Sprintf("Tokens: %s (%s in / %s out) | Cost: $%.4f",
		humanize.Comma(prompt+completion),
		humanize.Comma(prompt),
		humanize.Comma(completion),
		cost,
	)
}
