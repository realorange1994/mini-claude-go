package main

import (
	"errors"
	"testing"
)

func TestParseMaxTokensContextOverflowError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantOverflow int
		wantFound    bool
	}{
		{
			name:         "nil error",
			err:          nil,
			wantOverflow: 0,
			wantFound:    false,
		},
		{
			name:         "standard prompt too long",
			err:          errors.New("prompt is too long: 137500 tokens > 135000 maximum"),
			wantOverflow: 2500,
			wantFound:    true,
		},
		{
			name:         "request too large with max",
			err:          errors.New("request too large: 140000 tokens, max 135000"),
			wantOverflow: 5000,
			wantFound:    true,
		},
		{
			name:         "context length exceeded with token gap",
			err:          errors.New("context_length_exceeded: 150000 tokens > 135000"),
			wantOverflow: 15000,
			wantFound:    true,
		},
		{
			name:         "Chinese over max length",
			err:          errors.New("超过最大长度137500，最大135000"),
			wantOverflow: 2500,
			wantFound:    true,
		},
		{
			name:         "Chinese context length exceeded",
			err:          errors.New("上下文长度150000超过135000"),
			wantOverflow: 15000,
			wantFound:    true,
		},
		{
			name:         "context length error without numbers",
			err:          errors.New("context_length_exceeded"),
			wantOverflow: 0,
			wantFound:    false,
		},
		{
			name:         "unrelated error",
			err:          errors.New("rate_limit_error"),
			wantOverflow: 0,
			wantFound:    false,
		},
		{
			name:         "prompt too long with extra text",
			err:          errors.New("Error: prompt is too long: 200000 tokens > 128000 maximum context window"),
			wantOverflow: 72000,
			wantFound:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOverflow, gotFound := parseMaxTokensContextOverflowError(tt.err)
			if gotOverflow != tt.wantOverflow || gotFound != tt.wantFound {
				t.Errorf("parseMaxTokensContextOverflowError() = (%d, %v), want (%d, %v)",
					gotOverflow, gotFound, tt.wantOverflow, tt.wantFound)
			}
		})
	}
}
