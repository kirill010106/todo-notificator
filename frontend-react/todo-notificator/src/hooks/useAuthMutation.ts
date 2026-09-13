/* eslint-disable @typescript-eslint/no-explicit-any */
import { useMutation } from "@tanstack/react-query";
import { api } from "../api/api";
import { authStore } from "../stores/AuthStore";

export const useAuthMutation = () => {
  return useMutation({
    mutationFn: async () => {
      const path = authStore.authTab === "login" ? "/login" : "/register";
      const { data } = await api.post(path, authStore.formData);
      return data;
    },
    onSuccess: (data) => {
      if (authStore.authTab === "login") {
        localStorage.setItem("token", data.access_token);
        localStorage.setItem("refresh", data.refresh_token);
      } else {
        authStore.setMessage(`Регистрация успешна!`, "success");
        authStore.setAuthTab("login");
      }
    },
    onError: (error: any) => {
      let errorMsg = error.response?.data?.error || "Ошибка сервера";
      const status = error.response?.status;

      if (status === 400) errorMsg = "Некорректный email или пароль";
      if (status === 401) errorMsg = "Неверный логин или пароль";
      if (status === 409) errorMsg = "Этот email уже занят";

      authStore.setMessage(errorMsg, "error");
    },
  });
};
