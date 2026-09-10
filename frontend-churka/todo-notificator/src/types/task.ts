export type TaskStatus = "pending" | "done" | "burnt";

export interface Task {
  id: number;
  user_id: number;
  category_id: number | null;
  title: string;
  description: string;
  deadline: string | null;
  reminder_at: string | null;
  status: TaskStatus;
  is_notified: boolean;
  pomodoros_taken: number;
  reward_claimed: boolean;
}

export interface Pagination {
  limit: number;
  offset: number;
  total: number;
}

export interface ToastMessage {
  id: number;
  text: string; 
  type: "success" | "error";
}
