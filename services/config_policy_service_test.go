package services

import (
	"testing"

	"github.com/ablate-ai/RuleFlow/database"
)

func TestSanitizeExistingIDs(t *testing.T) {
	got := sanitizeExistingIDs([]int64{1, 2, 2, 3, 4}, func(id int64) bool {
		return id == 1 || id == 3
	})

	want := []int64{1, 3}
	if len(got) != len(want) {
		t.Fatalf("长度不匹配: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("结果不匹配: got=%v want=%v", got, want)
		}
	}
}

func TestSanitizeExistingIDsEmptyResultReturnsEmptySlice(t *testing.T) {
	got := sanitizeExistingIDs([]int64{10, 11}, func(int64) bool { return false })
	if got == nil {
		t.Fatalf("期望返回空切片而不是 nil")
	}
	if len(got) != 0 {
		t.Fatalf("期望空切片，实际 got=%v", got)
	}
}

func TestValidateConfigAfterSanitize(t *testing.T) {
	policy := &database.ConfigPolicy{
		Name:            "demo",
		Target:          "clash-mihomo",
		SubscriptionIDs: sanitizeExistingIDs([]int64{10, 11}, func(int64) bool { return false }),
		NodeIDs:         sanitizeExistingIDs([]int64{20}, func(int64) bool { return false }),
	}

	svc := &ConfigPolicyService{}
	if err := svc.ValidateConfig(policy); err == nil {
		t.Fatalf("期望清理后因无可用订阅源/节点而校验失败")
	}
}

func TestValidateConfigAllowsAllSubscriptionsWithoutExplicitSources(t *testing.T) {
	policy := &database.ConfigPolicy{
		Name:                     "all-subs",
		Target:                   "clash-mihomo",
		IncludeAllSubscriptions: true,
	}

	svc := &ConfigPolicyService{}
	if err := svc.ValidateConfig(policy); err != nil {
		t.Fatalf("包含所有订阅的策略不应因没有显式订阅而失败: %v", err)
	}
}

func TestSubscriptionIDsForPolicyDeduplicatesExplicitIDs(t *testing.T) {
	svc := &ConfigPolicyService{}
	ids, err := svc.subscriptionIDsForPolicy(nil, &database.ConfigPolicy{
		SubscriptionIDs: []int64{3, 3, 0, 7, 3},
	})
	if err != nil {
		t.Fatalf("subscriptionIDsForPolicy() error = %v", err)
	}
	want := []int64{3, 7}
	if len(ids) != len(want) {
		t.Fatalf("订阅去重结果长度错误: got=%v want=%v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("订阅去重结果错误: got=%v want=%v", ids, want)
		}
	}
}
