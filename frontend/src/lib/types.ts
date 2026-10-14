export interface Task {
  id: number;
  title: string;
  description: string;
  status: "pending" | "done" | "burnt";
  category_id?: number | null;
  deadline?: string | null;
  reminder_at?: string | null;
  pomodoros_taken?: number;
  reward_claimed?: boolean;
}
export interface Category {
  id: number;
  name: string;
}
export interface Session {
  id: number;
  task_id?: number | null;
  started_at: string;
  duration_minutes: number;
  breaks_used?: number;
  status: string;
}
export interface Quest {
  code: string;
  title: string;
  target: number;
  current: number;
  completed: boolean;
  reward_xp: number;
  claimed?: boolean;
}
export interface Gamification {
  daily_chest_claimed?: boolean;
  points: number;
  level: number;
  current_level_xp: number;
  next_level_xp: number;
  progress_percent: number;
  rank_title: string;
  streak: number;
  best_streak: number;
  daily_quests: Quest[];
  unlocked_count: number;
  total_achievements: number;
}
export interface User {
  timezone?: string;
  id: number;
  email: string;
  is_verified: boolean;
  is_premium: boolean;
  categories_count: number;
  active_reminders_count: number;
  categories_limit: number;
  reminders_limit: number;
}
export interface TaskPage {
  tasks?: Task[];
  pagination: { limit: number; offset: number; total: number };
}
export interface Bootstrap extends TaskPage {
  focus?: FocusSnapshot;
  user: User;
  categories: Category[];
  gamification?: Gamification;
  stats?: {
    total_pomodoros?: number;
    total_burnt_tasks?: number;
    [key: string]: unknown;
  };
  active_pomodoro?: Session | null;
}
export interface FocusSnapshot {
  server_now: string;
  phase: "idle" | "focus" | "extra_break" | "recovery_break";
  session: Session | null;
  phase_ends_at: string | null;
  remaining_work_seconds: number;
  completed_in_cycle: number;
  available_actions: string[];
}
export interface ActionResponse {
  operation_id: string;
  focus: FocusSnapshot;
  stats: Bootstrap["stats"];
  rewards: {
    task_xp: number;
    pomodoro_xp: number;
    achievement_xp: number;
    quest_xp: number;
    penalty_xp: number;
  };
  result: {
    pomodoro_counted: boolean;
    task_completed: boolean;
    changed_task_ids: number[];
    skipped_task_ids: number[];
  };
  unlocked_achievements: Achievement[];
}
export interface TodayState {
  server_now: string;
  timezone: string;
  today: { tasks: Task[]; pagination: TaskPage["pagination"] };
  overdue: { tasks: Task[]; pagination: TaskPage["pagination"] };
}
export interface Achievement {
  code: string;
  title: string;
  description: string;
  icon: string;
  unlocked: boolean;
  points: number;
}
export interface Activity {
  id?: string;
  action: string;
  timestamp?: string;
  created_at?: string;
  details?: Record<string, unknown>;
}
