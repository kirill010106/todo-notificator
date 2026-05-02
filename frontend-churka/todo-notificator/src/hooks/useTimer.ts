import { useState, useEffect, useRef, useCallback } from "react";

export const useTimer = (initialSeconds: number = 0, onExpire?: () => void) => {
  const [timeLeft, setTimeLeft] = useState(initialSeconds);
  const [isActive, setIsActive] = useState(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stop = useCallback(() => {
    if (intervalRef.current) clearInterval(intervalRef.current);
    intervalRef.current = null;
    setIsActive(false);
  }, []);

  const start = useCallback((seconds?: number) => {
    if (seconds !== undefined) setTimeLeft(seconds);
    setIsActive(true);
  }, []);

  useEffect(() => {
    if (isActive && timeLeft > 0) {
      intervalRef.current = setInterval(() => {
        setTimeLeft((prev) => prev - 1);
      }, 1000);
    } else if (timeLeft === 0 && isActive) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      stop();
      if (onExpire) onExpire();
    }

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [isActive, timeLeft, onExpire, stop]);

  const formatTime = useCallback(() => {
    const mins = Math.floor(timeLeft / 60)
      .toString()
      .padStart(2, "0");
    const secs = (timeLeft % 60).toString().padStart(2, "0");
    return `${mins}:${secs}`;
  }, [timeLeft]);

  return { timeLeft, isActive, start, stop, formatTime };
};
