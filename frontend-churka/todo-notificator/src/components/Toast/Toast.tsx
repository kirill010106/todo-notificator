// components/Toast/Toast.tsx
import React, { useEffect } from "react";
import "./Toast.scss";

export interface ToastMessage {
  id: number;
  text: string;
  type: "success" | "error";
}

interface ToastProps {
  toasts: ToastMessage[];
  onClose: (id: number) => void;
}

const Toast: React.FC<ToastProps> = ({ toasts, onClose }) => {
  return (
    <div className="toast-container">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} onClose={onClose} />
      ))}
    </div>
  );
};

const ToastItem = ({
  toast,
  onClose,
}: {
  toast: ToastMessage;
  onClose: (id: number) => void;
}) => {
  useEffect(() => {
    const timer = setTimeout(() => onClose(toast.id), 3000);
    return () => clearTimeout(timer);
  }, [toast.id, onClose]);

  return (
    <div className={`toast-item ${toast.type}`}>
      <span>{toast.type === "success" ? "✅" : "❌"}</span>
      <p>{toast.text}</p>
      <button onClick={() => onClose(toast.id)}>×</button>
    </div>
  );
};

export default Toast;
