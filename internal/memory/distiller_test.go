package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsMemoryMarker(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		want    bool
	}{
		{name: "i'll remember lowercase", give: "i'll remember that you prefer Go", want: true},
		{name: "I'll remember mixed case", give: "I'll remember that.", want: true},
		{name: "i will remember", give: "i will remember your preference", want: true},
		{name: "I will remember mixed case", give: "I Will Remember this.", want: true},
		{name: "regular assistant reply", give: "That sounds great! Go is an excellent choice.", want: false},
		{name: "partial match not marker", give: "You'll remember to do that.", want: false},
		{name: "empty string", give: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isMemoryMarker(tt.give))
		})
	}
}
