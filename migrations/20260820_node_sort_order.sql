ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS sort_order BIGINT NOT NULL DEFAULT 0;

-- Preserve the current grouping users see in the node page (KS before LA),
-- then leave newly imported nodes at the end until the user explicitly rearranges them.
WITH ranked AS (
    SELECT id, ROW_NUMBER() OVER (
        ORDER BY lower(split_part(name, '-', 1)), created_at DESC, id DESC
    ) AS position
    FROM nodes
)
UPDATE nodes AS n
SET sort_order = ranked.position
FROM ranked
WHERE n.id = ranked.id AND n.sort_order = 0;

CREATE INDEX IF NOT EXISTS idx_nodes_sort_order ON nodes(sort_order, created_at DESC, id DESC);

-- Policies used to select subscription sources. Convert those selections to the
-- concrete RuleFlow node records that exist now, so later subscription refreshes
-- and client output use the same explicit, drag-adjustable node list.
WITH selected_nodes AS (
    SELECT p.id AS policy_id, n.id AS node_id, n.sort_order
    FROM config_policies p
    JOIN nodes n ON (
        n.id = ANY(p.node_ids)
        OR n.source_id = ANY(p.subscription_ids)
        OR (
            p.include_all_subscriptions
            AND n.source_id IN (SELECT id FROM subscriptions WHERE enabled = true)
        )
    )
), policy_nodes AS (
    SELECT policy_id, array_agg(node_id ORDER BY sort_order, node_id) AS node_ids
    FROM selected_nodes
    GROUP BY policy_id
)
UPDATE config_policies p
SET node_ids = policy_nodes.node_ids,
    subscription_ids = '{}'::BIGINT[],
    include_all_subscriptions = false
FROM policy_nodes
WHERE p.id = policy_nodes.policy_id;
