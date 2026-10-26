package main

import (
	"strings"
	"testing"
)

func TestFormulaHintDoesNotRevealFinalAnswer(t *testing.T) {
	result, err := formulaHint(map[string]any{"expression": "2x + 3 = 9"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "方程") || !strings.Contains(result, "暂不计算最终答案") {
		t.Fatalf("unexpected hint: %q", result)
	}
	if strings.Contains(result, "x = 3") {
		t.Fatalf("hint leaked final answer: %q", result)
	}
}

func TestUnitConvert(t *testing.T) {
	result, err := unitConvert(map[string]any{"value": float64(250), "from": "cm", "to": "m"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "250 cm = 2.5 m" {
		t.Fatalf("unitConvert() = %q", result)
	}
}

func TestUnitConvertRejectsCrossDimension(t *testing.T) {
	if _, err := unitConvert(map[string]any{"value": float64(1), "from": "m", "to": "kg"}); err == nil {
		t.Fatal("unitConvert accepted cross-dimension conversion")
	}
}

func TestAuthorizationRequiresBearerToken(t *testing.T) {
	server := &server{token: "secret-token"}
	if !server.authorized("Bearer secret-token") {
		t.Fatal("valid token rejected")
	}
	if server.authorized("Bearer wrong-token") || server.authorized("secret-token") {
		t.Fatal("invalid token accepted")
	}
}
