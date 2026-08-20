package services

import (
	"testing"

	"github.com/ablate-ai/RuleFlow/database"
)

func TestPolicySelectionUsesOnlyRuleFlowNodes(t *testing.T) {
	policy := &database.ConfigPolicy{
		SubscriptionIDs: []int64{10, 20},
		NodeIDs:         []int64{7, 3, 9},
	}

	svc := &ConfigPolicyService{}
	svc.sanitizePolicyReferences(nil, policy)
	if len(policy.SubscriptionIDs) != 0 || policy.IncludeAllSubscriptions {
		t.Fatal("策略保存时必须清空订阅源选择")
	}
	if len(policy.NodeIDs) != 3 {
		t.Fatalf("节点选择不应被修改: %v", policy.NodeIDs)
	}
}
