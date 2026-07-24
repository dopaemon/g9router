package server

import "testing"

func TestEdgeVoiceModel(t *testing.T) {
	voice, ok := edgeVoiceModel("edge-tts/vi-VN-HoaiMyNeural")
	if !ok || voice != "vi-VN-HoaiMyNeural" {
		t.Fatalf("voice=%q ok=%v", voice, ok)
	}
	if _, ok := edgeVoiceModel("tts-1"); ok {
		t.Fatal("non-edge model was selected")
	}
}
