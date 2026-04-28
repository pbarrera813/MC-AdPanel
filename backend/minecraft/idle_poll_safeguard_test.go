package minecraft

import "testing"

func TestApplyListOnlineCountLocked_ActivatesAfterFiveConsecutiveZeros(t *testing.T) {
	rs := &runningServer{}
	for i := 0; i < emptyListSuppressionThreshold-1; i++ {
		applyListOnlineCountLocked(rs, 0)
		if rs.idlePollingSuppressed {
			t.Fatalf("expected suppression to stay disabled before threshold, got enabled at iteration %d", i+1)
		}
	}

	applyListOnlineCountLocked(rs, 0)
	if !rs.idlePollingSuppressed {
		t.Fatal("expected suppression to be enabled at threshold")
	}
	if rs.emptyListStreak != emptyListSuppressionThreshold {
		t.Fatalf("expected streak=%d, got %d", emptyListSuppressionThreshold, rs.emptyListStreak)
	}
}

func TestApplyListOnlineCountLocked_ResetsOnNonZeroCount(t *testing.T) {
	rs := &runningServer{}
	for i := 0; i < emptyListSuppressionThreshold; i++ {
		applyListOnlineCountLocked(rs, 0)
	}
	if !rs.idlePollingSuppressed {
		t.Fatal("expected suppression to be enabled before reset")
	}

	applyListOnlineCountLocked(rs, 3)
	if rs.idlePollingSuppressed {
		t.Fatal("expected suppression to be cleared after non-zero list result")
	}
	if rs.emptyListStreak != 0 {
		t.Fatalf("expected streak reset to 0, got %d", rs.emptyListStreak)
	}
}

func TestResetIdlePollingSafeguardLocked_ClearsSuppressionState(t *testing.T) {
	rs := &runningServer{
		emptyListStreak:       emptyListSuppressionThreshold,
		idlePollingSuppressed: true,
	}

	resetIdlePollingSafeguardLocked(rs)
	if rs.idlePollingSuppressed {
		t.Fatal("expected suppression to be disabled after reset")
	}
	if rs.emptyListStreak != 0 {
		t.Fatalf("expected streak reset to 0, got %d", rs.emptyListStreak)
	}
}
