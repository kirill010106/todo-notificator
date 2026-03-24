INSERT INTO user_stats (user_id, points, level, total_pomodoros, total_burnt_tasks, current_streak, best_streak, updated_at)
SELECT 
    id, 
    0, 
    1, 
    0, 
    0, 
    0, 
    0, 
    NOW()
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM user_stats us WHERE us.user_id = u.id
);