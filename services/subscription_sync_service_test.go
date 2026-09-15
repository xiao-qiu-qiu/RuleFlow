package services

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ablate-ai/RuleFlow/database"
)

type remappingPolicyRepo struct {
	policies []*database.ConfigPolicy
	remapErr error
}

func (r *remappingPolicyRepo) List(context.Context) ([]*database.ConfigPolicy, error) {
	return r.policies, nil
}

func (r *remappingPolicyRepo) RemapNodeIDs(_ context.Context, replacements map[int64]int64) error {
	if r.remapErr != nil {
		return r.remapErr
	}

	for _, policy := range r.policies {
		remapped := make([]int64, 0, len(policy.NodeIDs))
		for _, id := range policy.NodeIDs {
			newID, replaced := replacements[id]
			if replaced {
				if newID > 0 {
					remapped = append(remapped, newID)
				}
				continue
			}
			remapped = append(remapped, id)
		}
		policy.NodeIDs = remapped
	}

	return nil
}

type recordingPolicyCache struct {
	deletedTokens []string
}

func (c *recordingPolicyCache) DeletePolicyConfig(_ context.Context, token string) error {
	c.deletedTokens = append(c.deletedTokens, token)
	return nil
}

func TestPolicyReferencesSubscription(t *testing.T) {
	policy := &database.ConfigPolicy{
		ID:              1,
		Token:           "demo-token",
		SubscriptionIDs: []int64{1001, 1002, 1003},
	}

	if !policyReferencesSubscription(policy, 1002) {
		t.Fatalf("期望命中关联订阅")
	}

	if policyReferencesSubscription(policy, 2001) {
		t.Fatalf("不期望命中未关联订阅")
	}

	if policyReferencesSubscription(nil, 1002) {
		t.Fatalf("nil 策略不应命中")
	}

	policy.IncludeAllSubscriptions = true
	if !policyReferencesSubscription(policy, 2001) {
		t.Fatalf("包含所有订阅的策略应命中任意订阅")
	}
}

func TestRemapNodePoliciesInvalidatesTokensCollectedBeforeRemap(t *testing.T) {
	repo := &remappingPolicyRepo{policies: []*database.ConfigPolicy{
		{ID: 1, Token: "remapped-token", NodeIDs: []int64{100, 300}},
		{ID: 2, Token: "removed-token", NodeIDs: []int64{200}},
		{ID: 3, Token: "unrelated-token", NodeIDs: []int64{400}},
	}}
	policyCache := &recordingPolicyCache{}
	svc := &SubscriptionSyncService{
		policyRepo:  repo,
		policyCache: policyCache,
	}

	err := svc.remapNodePolicies(context.Background(), map[int64]int64{
		100: 101,
		200: 0,
	})
	if err != nil {
		t.Fatalf("remapNodePolicies() error = %v", err)
	}

	if got, want := repo.policies[0].NodeIDs, []int64{101, 300}; !slices.Equal(got, want) {
		t.Fatalf("重映射后的节点 ID = %v，期望 %v", got, want)
	}
	if len(repo.policies[1].NodeIDs) != 0 {
		t.Fatalf("已移除节点仍保留在策略中: %v", repo.policies[1].NodeIDs)
	}
	if got, want := policyCache.deletedTokens, []string{"remapped-token", "removed-token"}; !slices.Equal(got, want) {
		t.Fatalf("失效 token = %v，期望 %v", got, want)
	}
}

func TestRemapNodePoliciesDoesNotInvalidateCacheWhenRemapFails(t *testing.T) {
	repo := &remappingPolicyRepo{
		policies: []*database.ConfigPolicy{
			{ID: 1, Token: "unchanged-token", NodeIDs: []int64{100}},
		},
		remapErr: errors.New("remap failed"),
	}
	policyCache := &recordingPolicyCache{}
	svc := &SubscriptionSyncService{
		policyRepo:  repo,
		policyCache: policyCache,
	}

	err := svc.remapNodePolicies(context.Background(), map[int64]int64{100: 101})
	if err == nil {
		t.Fatal("期望节点重映射失败")
	}
	if len(policyCache.deletedTokens) != 0 {
		t.Fatalf("重映射失败后不应清缓存，实际失效 token: %v", policyCache.deletedTokens)
	}
}
