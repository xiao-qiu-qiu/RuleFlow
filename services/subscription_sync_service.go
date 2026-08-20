package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ablate-ai/RuleFlow/cache"
	"github.com/ablate-ai/RuleFlow/database"
	"github.com/ablate-ai/RuleFlow/internal/app"
)

type policyCacheInvalidator interface {
	DeletePolicyConfig(ctx context.Context, token string) error
}

type preparedSubscriptionSync struct {
	sub      *database.Subscription
	nodes    []*app.ProxyNode
	userInfo *database.UserInfo
	err      error
}

// SubscriptionBatchSyncFailure 批量同步失败项
type SubscriptionBatchSyncFailure struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// SubscriptionBatchSyncResult 批量同步结果
type SubscriptionBatchSyncResult struct {
	Total        int                            `json:"total"`
	Attempted    int                            `json:"attempted"`
	Succeeded    int                            `json:"succeeded"`
	Failed       int                            `json:"failed"`
	Skipped      int                            `json:"skipped"`
	TotalNodes   int                            `json:"total_nodes"`
	Failures     []SubscriptionBatchSyncFailure `json:"failures"`
	SkippedNames []string                       `json:"skipped_names"`
}

// SubscriptionSyncService 订阅同步服务
type SubscriptionSyncService struct {
	subRepo     *database.SubscriptionRepo
	nodeRepo    *database.NodeRepo
	policyRepo  *database.ConfigPolicyRepo
	policyCache policyCacheInvalidator
}

// NewSubscriptionSyncService 创建订阅同步服务
func NewSubscriptionSyncService(
	subRepo *database.SubscriptionRepo,
	nodeRepo *database.NodeRepo,
	policyRepo *database.ConfigPolicyRepo,
	policyCache *cache.SubscriptionCache,
) *SubscriptionSyncService {
	return &SubscriptionSyncService{
		subRepo:     subRepo,
		nodeRepo:    nodeRepo,
		policyRepo:  policyRepo,
		policyCache: policyCache,
	}
}

// SyncSubscription 同步指定订阅的节点
// 使用完全替换策略：删除该订阅的旧节点，插入新节点
func (s *SubscriptionSyncService) SyncSubscription(ctx context.Context, subscriptionID int64) (int, error) {
	log.Printf("[sync] 开始同步订阅: id=%d", subscriptionID)

	sub, err := s.subRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return 0, fmt.Errorf("订阅不存在: %d", subscriptionID)
	}

	prepared := s.prepareSubscriptionSync(ctx, sub)
	return s.applyPreparedSubscriptionSync(ctx, prepared)
}

func (s *SubscriptionSyncService) invalidateRelatedPolicyCaches(ctx context.Context, subscriptionID int64) {
	if s.policyRepo == nil || s.policyCache == nil {
		return
	}

	policies, err := s.policyRepo.List(ctx)
	if err != nil {
		log.Printf("[sync] 查询关联配置策略失败: subscription_id=%d err=%v", subscriptionID, err)
		return
	}

	invalidated := 0
	for _, policy := range policies {
		if !policyReferencesSubscription(policy, subscriptionID) {
			continue
		}
		token := strings.TrimSpace(policy.Token)
		if token == "" {
			continue
		}
		if err := s.policyCache.DeletePolicyConfig(ctx, token); err != nil {
			log.Printf("[sync] 清理配置缓存失败: subscription_id=%d policy_id=%d token=%s err=%v", subscriptionID, policy.ID, token, err)
			continue
		}
		invalidated++
	}

	if invalidated > 0 {
		log.Printf("[sync] 已清理关联配置缓存: subscription_id=%d count=%d", subscriptionID, invalidated)
	}
}

func (s *SubscriptionSyncService) invalidateNodePolicyCaches(ctx context.Context, replacements map[int64]int64) {
	if len(replacements) == 0 || s.policyRepo == nil || s.policyCache == nil {
		return
	}
	policies, err := s.policyRepo.List(ctx)
	if err != nil {
		log.Printf("[sync] 查询节点策略缓存失败: err=%v", err)
		return
	}
	for _, policy := range policies {
		matched := false
		for _, nodeID := range policy.NodeIDs {
			for oldID := range replacements {
				if nodeID == oldID {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched && strings.TrimSpace(policy.Token) != "" {
			if err := s.policyCache.DeletePolicyConfig(ctx, policy.Token); err != nil {
				log.Printf("[sync] 清理节点策略缓存失败: policy_id=%d err=%v", policy.ID, err)
			}
		}
	}
}

func policyReferencesSubscription(policy *database.ConfigPolicy, subscriptionID int64) bool {
	if policy == nil {
		return false
	}
	if policy.IncludeAllSubscriptions {
		return true
	}
	for _, id := range policy.SubscriptionIDs {
		if id == subscriptionID {
			return true
		}
	}
	return false
}

// parseUserInfo 解析 Subscription-Userinfo 响应头
// 格式：upload=1234; download=5678; total=10000000000; expire=1750000000
func parseUserInfo(header string) *database.UserInfo {
	info := &database.UserInfo{}
	found := false
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "upload":
			info.Upload = val
			found = true
		case "download":
			info.Download = val
			found = true
		case "total":
			info.Total = val
			found = true
		case "expire":
			info.Expire = &val
		}
	}
	if !found {
		return nil
	}
	return info
}

// fetchSubscriptionContent 从 URL 拉取订阅内容，同时返回 Subscription-Userinfo 响应头
func (s *SubscriptionSyncService) fetchSubscriptionContent(ctx context.Context, url string) (content string, userInfoHeader string, err error) {
	content, headers, err := app.FetchSubscriptionContent(ctx, url)
	if err != nil {
		return "", "", err
	}

	return content, headers.Get("Subscription-Userinfo"), nil
}

func (s *SubscriptionSyncService) prepareSubscriptionSync(ctx context.Context, sub *database.Subscription) preparedSubscriptionSync {
	if sub == nil {
		return preparedSubscriptionSync{err: fmt.Errorf("订阅不存在")}
	}
	if !sub.Enabled {
		return preparedSubscriptionSync{sub: sub, err: fmt.Errorf("订阅已禁用: %s", sub.Name)}
	}
	if sub.URL == nil || strings.TrimSpace(*sub.URL) == "" {
		return preparedSubscriptionSync{sub: sub, err: fmt.Errorf("订阅没有配置 URL: %s", sub.Name)}
	}

	log.Printf("[sync] 拉取订阅内容: id=%d url=%s", sub.ID, *sub.URL)
	content, userInfoHeader, err := s.fetchSubscriptionContent(ctx, *sub.URL)
	if err != nil {
		log.Printf("[sync] 拉取失败: id=%d err=%v", sub.ID, err)
		return preparedSubscriptionSync{sub: sub, err: fmt.Errorf("拉取订阅内容失败: %w", err)}
	}
	log.Printf("[sync] 拉取成功: id=%d content_length=%d", sub.ID, len(content))

	nodes, err := app.ParseSubscription(content)
	if err != nil {
		log.Printf("[sync] 解析失败: id=%d err=%v", sub.ID, err)
		return preparedSubscriptionSync{sub: sub, err: fmt.Errorf("解析订阅内容失败: %w", err)}
	}
	if len(nodes) == 0 {
		log.Printf("[sync] 未解析到任何节点: id=%d", sub.ID)
		return preparedSubscriptionSync{sub: sub, err: fmt.Errorf("订阅中没有有效节点")}
	}

	return preparedSubscriptionSync{
		sub:      sub,
		nodes:    nodes,
		userInfo: parseUserInfo(userInfoHeader),
	}
}

func (s *SubscriptionSyncService) applyPreparedSubscriptionSync(ctx context.Context, prepared preparedSubscriptionSync) (int, error) {
	if prepared.sub == nil {
		if prepared.err != nil {
			return 0, prepared.err
		}
		return 0, fmt.Errorf("订阅不存在")
	}

	if prepared.err != nil {
		_ = s.subRepo.UpdateFetchResultByID(ctx, prepared.sub.ID, 0, prepared.err)
		return 0, prepared.err
	}

	// 保留同步前的全局节点顺序。订阅同步会替换节点记录，之后按业务身份
	// 把仍然存在的节点放回原位置，避免定时刷新破坏用户的拖拽排序。
	previousNodes, err := s.nodeRepo.List(ctx, database.NodeFilter{})
	if err != nil {
		return 0, fmt.Errorf("读取同步前节点顺序失败: %w", err)
	}

	namePrefix := ""
	if !prepared.sub.DisableNamePrefix {
		namePrefix = fmt.Sprintf("%s-", strings.TrimSpace(prepared.sub.Name))
	}
	deleted, err := s.nodeRepo.DeleteBySourceID(ctx, prepared.sub.ID)
	if err != nil {
		return 0, fmt.Errorf("删除旧节点失败: %w", err)
	}
	log.Printf("[sync] 已删除旧节点: id=%d count=%d", prepared.sub.ID, deleted)

	dbNodes := s.convertToDBNodes(prepared.nodes, prepared.sub.ID, namePrefix)
	if err := s.nodeRepo.BatchCreate(ctx, dbNodes); err != nil {
		log.Printf("[sync] 插入新节点失败: id=%d err=%v", prepared.sub.ID, err)
		return 0, fmt.Errorf("插入新节点失败: %w", err)
	}
	replacements, err := s.restoreSubscriptionNodeOrder(ctx, prepared.sub.ID, previousNodes)
	if err != nil {
		return 0, fmt.Errorf("恢复节点顺序失败: %w", err)
	}
	if s.policyRepo != nil {
		if err := s.policyRepo.RemapNodeIDs(ctx, replacements); err != nil {
			return 0, fmt.Errorf("更新策略节点引用失败: %w", err)
		}
	}
	s.invalidateNodePolicyCaches(ctx, replacements)

	now := time.Now()
	nodeCount := len(dbNodes)
	_ = s.subRepo.UpdateFetchResultByID(ctx, prepared.sub.ID, nodeCount, nil)

	if err := s.updateNodesLastSyncedAt(ctx, prepared.sub.ID, now); err != nil {
		_ = err
	}

	if err := s.subRepo.UpdateUserInfoByID(ctx, prepared.sub.ID, prepared.userInfo); err != nil {
		log.Printf("[sync] 保存流量信息失败: id=%d err=%v", prepared.sub.ID, err)
	} else if prepared.userInfo != nil {
		log.Printf("[sync] 流量信息已保存: id=%d upload=%d download=%d total=%d", prepared.sub.ID, prepared.userInfo.Upload, prepared.userInfo.Download, prepared.userInfo.Total)
	}

	s.invalidateRelatedPolicyCaches(ctx, prepared.sub.ID)
	log.Printf("[sync] 同步完成: %s，节点数: %d", prepared.sub.Name, nodeCount)
	return nodeCount, nil
}

func nodeIdentity(node database.Node) string {
	return fmt.Sprintf("%s\x00%s\x00%d", node.Name, node.Server, node.Port)
}

func (s *SubscriptionSyncService) restoreSubscriptionNodeOrder(ctx context.Context, sourceID int64, previousNodes []database.Node) (map[int64]int64, error) {
	currentNodes, err := s.nodeRepo.List(ctx, database.NodeFilter{})
	if err != nil {
		return nil, err
	}

	currentByIdentity := make(map[string]database.Node)
	for _, node := range currentNodes {
		if node.SourceID != nil && *node.SourceID == sourceID {
			currentByIdentity[nodeIdentity(node)] = node
		}
	}

	ordered := make([]database.Node, 0, len(currentNodes))
	replacements := make(map[int64]int64)
	seen := make(map[int64]struct{}, len(currentNodes))
	for _, previous := range previousNodes {
		if previous.SourceID != nil && *previous.SourceID == sourceID {
			if replacement, ok := currentByIdentity[nodeIdentity(previous)]; ok {
				ordered = append(ordered, replacement)
				seen[replacement.ID] = struct{}{}
				replacements[previous.ID] = replacement.ID
			}
			continue
		}
		for _, current := range currentNodes {
			if current.ID == previous.ID {
				ordered = append(ordered, current)
				seen[current.ID] = struct{}{}
				break
			}
		}
	}

	newNodes := make([]database.Node, 0)
	for _, current := range currentNodes {
		if _, ok := seen[current.ID]; ok {
			continue
		}
		newNodes = append(newNodes, current)
	}
	sort.SliceStable(newNodes, func(i, j int) bool {
		if newNodes[i].SortOrder != newNodes[j].SortOrder {
			return newNodes[i].SortOrder < newNodes[j].SortOrder
		}
		return newNodes[i].ID < newNodes[j].ID
	})
	ordered = append(ordered, newNodes...)

	ids := make([]int64, 0, len(ordered))
	for _, node := range ordered {
		ids = append(ids, node.ID)
	}
	if err := s.nodeRepo.UpdateOrder(ctx, ids); err != nil {
		return nil, err
	}
	return replacements, nil
}

// convertToDBNodes 将解析的节点转换为数据库模型
func (s *SubscriptionSyncService) convertToDBNodes(nodes []*app.ProxyNode, sourceID int64, namePrefix string) []database.Node {
	dbNodes := make([]database.Node, 0, len(nodes))

	for _, node := range nodes {
		name := node.Name
		if namePrefix != "" {
			name = namePrefix + name
		}
		dbNode := database.Node{
			Name:     name,
			Protocol: node.Protocol,
			Server:   node.Server,
			Port:     node.Port,
			Config:   node.Options,
			SourceID: &sourceID,
			Enabled:  true,
			Tags:     []string{},
		}
		dbNodes = append(dbNodes, dbNode)
	}

	return dbNodes
}

// updateNodesLastSyncedAt 更新节点的最后同步时间
func (s *SubscriptionSyncService) updateNodesLastSyncedAt(ctx context.Context, sourceID int64, syncTime time.Time) error {
	// 这个方法可以扩展为批量更新操作
	// 目前在插入时已经设置了 created_at，这里可以添加一个专门的方法
	// 暂时保留为空实现，因为 created_at 可以作为同步时间的参考
	return nil
}

// GetSyncStatus 获取订阅同步状态
func (s *SubscriptionSyncService) GetSyncStatus(ctx context.Context, subscriptionID int64) (map[string]interface{}, error) {
	// 获取订阅信息
	sub, err := s.subRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("订阅不存在: %d", subscriptionID)
	}
	// 获取节点数量
	nodeCount, err := s.nodeRepo.CountBySourceID(ctx, sub.ID)
	if err != nil {
		return nil, fmt.Errorf("获取节点数量失败: %w", err)
	}

	status := map[string]interface{}{
		"subscription_id":   sub.ID,
		"subscription_name": sub.Name,
		"enabled":           sub.Enabled,
		"node_count":        nodeCount,
		"last_fetched_at":   sub.LastFetchedAt,
		"last_fetch_error":  sub.LastFetchError,
	}

	return status, nil
}

// SyncAllSubscriptions 同步所有启用的订阅，采用两阶段流程：
// 1. 并行拉取和解析
// 2. 串行落库，减少数据库写冲突
func (s *SubscriptionSyncService) SyncAllSubscriptions(ctx context.Context) (*SubscriptionBatchSyncResult, error) {
	subs, err := s.subRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取订阅列表失败: %w", err)
	}

	result := &SubscriptionBatchSyncResult{
		Total:        len(subs),
		Failures:     make([]SubscriptionBatchSyncFailure, 0),
		SkippedNames: make([]string, 0),
	}

	targets := make([]database.Subscription, 0, len(subs))
	for _, sub := range subs {
		if !sub.Enabled {
			result.SkippedNames = append(result.SkippedNames, sub.Name)
			continue
		}
		if sub.URL == nil || strings.TrimSpace(*sub.URL) == "" {
			result.SkippedNames = append(result.SkippedNames, sub.Name)
			continue
		}
		targets = append(targets, sub)
	}
	result.Attempted = len(targets)
	result.Skipped = len(result.SkippedNames)

	prepared := make([]preparedSubscriptionSync, len(targets))
	var wg sync.WaitGroup
	wg.Add(len(targets))
	for idx, sub := range targets {
		idx := idx
		sub := sub
		go func() {
			defer wg.Done()
			prepared[idx] = s.prepareSubscriptionSync(ctx, &sub)
		}()
	}
	wg.Wait()

	for _, item := range prepared {
		count, err := s.applyPreparedSubscriptionSync(ctx, item)
		if err != nil {
			result.Failed++
			result.Failures = append(result.Failures, SubscriptionBatchSyncFailure{
				ID:    item.sub.ID,
				Name:  item.sub.Name,
				Error: err.Error(),
			})
			continue
		}
		result.Succeeded++
		result.TotalNodes += count
	}

	return result, nil
}
