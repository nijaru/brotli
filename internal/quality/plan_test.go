package quality

import (
	"testing"
)

func TestNewPlan(t *testing.T) {
	tests := []struct {
		quality     int
		lgwin       int
		wantTier    Tier
		wantStatic  bool
		wantBlock   bool
		wantContext bool
		wantHQCtx   bool
		wantHQBlk   bool
	}{
		{0, 22, TierQ0, true, false, false, false, false},
		{1, 22, TierQ1, true, false, false, false, false},
		{2, 22, TierGeneric, true, false, false, false, false},
		{3, 22, TierGeneric, false, false, false, false, false},
		{4, 22, TierGeneric, false, true, false, false, false},
		{5, 22, TierGeneric, false, true, true, false, false},
		{6, 22, TierGeneric, false, true, true, false, false},
		{7, 22, TierGeneric, false, true, true, true, false},
		{8, 22, TierGeneric, false, true, true, true, false},
		{9, 22, TierGeneric, false, true, true, true, false},
		{10, 22, TierZopfli, false, true, true, true, true},
		{11, 22, TierZopfli, false, true, true, true, true},
	}

	for _, tt := range tests {
		p := NewPlan(tt.quality, tt.lgwin, 0, 0, false)
		if p.Tier != tt.wantTier {
			t.Errorf("Q%d: tier=%v, want %v", tt.quality, p.Tier, tt.wantTier)
		}
		if p.StaticEntropy != tt.wantStatic {
			t.Errorf("Q%d: staticEntropy=%v, want %v", tt.quality, p.StaticEntropy, tt.wantStatic)
		}
		if p.BlockSplit != tt.wantBlock {
			t.Errorf("Q%d: blockSplit=%v, want %v", tt.quality, p.BlockSplit, tt.wantBlock)
		}
		if p.ContextModeling != tt.wantContext {
			t.Errorf("Q%d: contextModeling=%v, want %v", tt.quality, p.ContextModeling, tt.wantContext)
		}
		if p.HQContext != tt.wantHQCtx {
			t.Errorf("Q%d: hqContext=%v, want %v", tt.quality, p.HQContext, tt.wantHQCtx)
		}
		if p.HQBlockSplit != tt.wantHQBlk {
			t.Errorf("Q%d: hqBlockSplit=%v, want %v", tt.quality, p.HQBlockSplit, tt.wantHQBlk)
		}
	}
}

func TestZopfliParams(t *testing.T) {
	p10 := NewPlan(10, 22, 0, 0, false)
	if p10.MaxZopfliLen != 150 {
		t.Errorf("Q10 maxZopfliLen=%d, want 150", p10.MaxZopfliLen)
	}
	if p10.ZopfliCandidates != 1 {
		t.Errorf("Q10 zopfliCandidates=%d, want 1", p10.ZopfliCandidates)
	}

	p11 := NewPlan(11, 22, 0, 0, false)
	if p11.MaxZopfliLen != 325 {
		t.Errorf("Q11 maxZopfliLen=%d, want 325", p11.MaxZopfliLen)
	}
	if p11.ZopfliCandidates != 5 {
		t.Errorf("Q11 zopfliCandidates=%d, want 5", p11.ZopfliCandidates)
	}
}

func TestLgblock(t *testing.T) {
	// Q0/Q1 use full window
	p0 := NewPlan(0, 22, 0, 0, false)
	if p0.Lgblock != 22 {
		t.Errorf("Q0 lgblock=%d, want 22", p0.Lgblock)
	}

	// Q3 uses lgblock=14
	p3 := NewPlan(3, 22, 0, 0, false)
	if p3.Lgblock != 14 {
		t.Errorf("Q3 lgblock=%d, want 14", p3.Lgblock)
	}

	// Q4 uses lgblock=16
	p4 := NewPlan(4, 22, 0, 0, false)
	if p4.Lgblock != 16 {
		t.Errorf("Q4 lgblock=%d, want 16", p4.Lgblock)
	}

	// Q9 with lgwin=22 uses lgblock=18
	p9 := NewPlan(9, 22, 0, 0, false)
	if p9.Lgblock != 18 {
		t.Errorf("Q9 lgblock=%d, want 18", p9.Lgblock)
	}
}

func TestLargeWindow(t *testing.T) {
	// Q0 cannot use large window
	p0 := NewPlan(0, 22, 0, 0, true)
	if p0.LargeWindow {
		t.Error("Q0 should not allow large window")
	}

	// Q3+ can use large window
	p3 := NewPlan(3, 22, 0, 0, true)
	if !p3.LargeWindow {
		t.Error("Q3 should allow large window")
	}
}
