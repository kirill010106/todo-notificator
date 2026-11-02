-- +goose Up
-- Stop the previous backend before applying: it must not sell/consume boosters.
-- Preserve a snapshot even after the compensation and future wallet writes.
CREATE TABLE IF NOT EXISTS retired_boosters (
 user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 inventory JSONB NOT NULL,
 active BOOLEAN NOT NULL,
 refund BIGINT NOT NULL CHECK (refund >= 0),
 retired_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO retired_boosters(user_id,inventory,active,refund)
SELECT user_id, COALESCE(data->'boosters','{}'::jsonb),
 COALESCE((data->>'activeBooster')::boolean,false),
 (GREATEST(COALESCE((data->'boosters'->>'xp_boost')::bigint,0),0)
  + CASE WHEN COALESCE((data->>'activeBooster')::boolean,false) THEN 1 ELSE 0 END) * 100
FROM reward_wallets ON CONFLICT DO NOTHING;
WITH credited AS (
 INSERT INTO reward_ledger(user_id,event_key,coins,description)
 SELECT user_id,'retire:xp_boost:v1',refund,'Возврат за неиспользованные усилители опыта'
 FROM retired_boosters WHERE refund > 0
 ON CONFLICT(user_id,event_key) DO NOTHING RETURNING user_id,coins
)
UPDATE reward_wallets w SET data=jsonb_set(w.data,'{coins}',to_jsonb((w.data->>'coins')::bigint+c.coins))
FROM credited c WHERE c.user_id=w.user_id;
UPDATE reward_wallets SET data=jsonb_set(
 jsonb_set(data,'{boosters}',COALESCE(data->'boosters','{}'::jsonb)-'xp_boost'),
 '{activeBooster}','false'::jsonb);

-- Existing completed/rewarded tasks cannot regain the reward on reopening.
-- Reconstruct only evidence still present in the database, never XP estimates.
INSERT INTO daily_task_activity(user_id,activity_date,task_id,action,recorded_at)
SELECT t.user_id,(t.updated_at AT TIME ZONE u.timezone)::date,t.id,'completed',t.updated_at
FROM tasks t JOIN users u ON u.id=t.user_id
WHERE (t.reward_claimed OR t.status='done')
 AND NOT EXISTS (SELECT 1 FROM daily_task_activity a WHERE a.user_id=t.user_id AND a.task_id=t.id AND a.action='completed')
ON CONFLICT DO NOTHING;
UPDATE tasks t SET reward_claimed=true WHERE NOT reward_claimed AND
 EXISTS (SELECT 1 FROM daily_task_activity a WHERE a.user_id=t.user_id AND a.task_id=t.id AND a.action='completed');
CREATE INDEX IF NOT EXISTS task_completion_history_idx
 ON daily_task_activity(user_id,task_id) WHERE action='completed';

-- Unlock eligible awards without changing XP, balances, or existing timestamps.
INSERT INTO achievements(user_id,code)
SELECT a.user_id,v.code FROM (
 SELECT user_id,COUNT(DISTINCT task_id) AS total FROM daily_task_activity WHERE action='completed' GROUP BY user_id
) a CROSS JOIN (VALUES ('first_step',1),('task_spree',10),('task_centurion',50)) v(code,target)
WHERE a.total>=v.target ON CONFLICT(user_id,code) DO NOTHING;
INSERT INTO achievements(user_id,code)
SELECT s.user_id,v.code FROM user_stats s
CROSS JOIN (VALUES ('pomodoro_rookie',1),('pomodoro_guru',20),('pomodoro_master',100)) v(code,target)
WHERE s.total_pomodoros>=v.target ON CONFLICT(user_id,code) DO NOTHING;

-- +goose Down
-- Financial compensation cannot safely be undone after coins have been spent.
-- Keep history and the refund marker; roll forward with a corrective migration.
SELECT 1;
