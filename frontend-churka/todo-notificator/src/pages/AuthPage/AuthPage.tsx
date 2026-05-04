<<<<<<< Updated upstream
import React from "react";
import { useNavigate } from "react-router-dom";
import { observer } from "mobx-react-lite";
import { authStore } from "../../stores/AuthStore";
import { useAuthMutation } from "../../hooks/useAuthMutation";
import "./AuthPage.scss";

const AuthPage: React.FC = observer(() => {
  const navigate = useNavigate();
  const { mutate, isPending } = useAuthMutation();

  const handleFormSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    mutate(undefined, {
      onSuccess: () => {
        if (authStore.authTab === "login") {
          navigate("/main");
        } else {
          authStore.setAuthTab("login");
        }
      },
    });
=======
import React, { useState } from "react";
import "./AuthPage.css";

const API_BASE = "http://localhost:8082/api/v1";

type AuthTab = "login" | "register";

const AuthPage: React.FC = () => {
  const [authTab, setAuthTab] = useState<AuthTab>("login");
  const [loading, setLoading] = useState(false);
  const [formData, setFormData] = useState({ email: "", password: "" });
  const [showPassword, setShowPassword] = useState(false);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  const getPasswordStrength = (pass: string): number => {
    if (!pass) return 0;
    let strength = 0;
    if (pass.length >= 8) strength++;
    if (/[A-Z]/.test(pass)) strength++;
    if (/[0-9]/.test(pass)) strength++;
    if (/[^A-Za-z0-9]/.test(pass)) strength++;
    return strength;
  };

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setLoading(true);

    // В Swagger у тебя /login и /register без префикса /auth
    const path = authTab === "login" ? "/login" : "/register";

    try {
      const response = await fetch(`${API_BASE}${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(formData),
      });

      const data = await response.json();

      if (!response.ok) {
        // Обработка специфичных ошибок из твоего Swagger
        if (response.status === 401)
          throw new Error("Неверный логин или пароль");
        if (response.status === 409)
          throw new Error("Пользователь уже существует");
        throw new Error(data.error || "Ошибка сервера");
      }

      if (authTab === "login") {
        // Согласно Swagger: access_token, refresh_token (snake_case)
        if (data.access_token) {
          localStorage.setItem("token", data.access_token);
          localStorage.setItem("refresh", data.refresh_token);

          alert("Вход выполнен!");
          window.location.href = "/dashboard";
        }
      } else {
        // Для register возвращается user_id
        alert(`Регистрация успешна! ID пользователя: ${data.user_id}`);
        setAuthTab("login");
        setFormData((prev) => ({ ...prev, password: "" }));
      }
    } catch (error: any) {
      alert(`Ошибка: ${error.message}`);
    } finally {
      setLoading(false);
    }
>>>>>>> Stashed changes
  };

  return (
    <div className="auth-container">
      <div className="auth-card">
        <header className="auth-header">
          <div className="auth-logo">✅</div>
          <h1>ToDo Notificator</h1>
<<<<<<< Updated upstream
        </header>

        <nav className="auth-tabs">
          {(["login", "register"] as const).map((tab) => (
            <button
              key={tab}
              className={`tab-btn ${authStore.authTab === tab ? "active" : ""}`}
              onClick={() => authStore.setAuthTab(tab)}
=======
          <p>Управляй задачами эффективно</p>
        </header>

        <nav className="auth-tabs">
          {(["login", "register"] as AuthTab[]).map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => {
                setAuthTab(tab);
                setFormData({ email: "", password: "" });
              }}
              className={`tab-btn ${authTab === tab ? "active" : ""}`}
>>>>>>> Stashed changes
            >
              {tab === "login" ? "Войти" : "Регистрация"}
            </button>
          ))}
        </nav>

<<<<<<< Updated upstream
        <form onSubmit={handleFormSubmit} className="auth-form">
          {authStore.message && (
            <div className={`auth-notification ${authStore.message.type}`}>
              {authStore.message.text}
            </div>
          )}

          <div className="input-group">
            <label>Email</label>
            <input
              name="email"
              type="email"
              required
              value={authStore.formData.email}
              onChange={(e) => authStore.setFormField("email", e.target.value)}
=======
        <form onSubmit={handleSubmit} className="auth-form">
          <div className="input-group">
            <label htmlFor="email">Email</label>
            <input
              id="email"
              name="email"
              type="email"
              placeholder="you@example.com"
              required
              value={formData.email}
              onChange={handleChange}
>>>>>>> Stashed changes
            />
          </div>

          <div className="input-group">
<<<<<<< Updated upstream
            <label>Пароль</label>
            <div className="password-wrapper">
              <input
                name="password"
                type={authStore.showPassword ? "text" : "password"}
                required
                value={authStore.formData.password}
                onChange={(e) =>
                  authStore.setFormField("password", e.target.value)
                }
              />
              <button
                type="button"
                onClick={() => authStore.togglePasswordVisibility()}
              >
                {authStore.showPassword ? "🙈" : "👁"}
              </button>
            </div>

            {authStore.authTab === "register" &&
              authStore.formData.password.length > 0 && (
                <div className="strength-meter">
                  {[1, 2, 3, 4].map((step) => (
                    <div
                      key={step}
                      className={`strength-step ${authStore.passwordStrength >= step ? "filled" : ""}`}
                    />
                  ))}
                </div>
              )}
=======
            <label htmlFor="password">Пароль</label>
            <div className="password-wrapper">
              <input
                id="password"
                name="password"
                type={showPassword ? "text" : "password"}
                placeholder={
                  authTab === "login" ? "••••••••" : "Минимум 8 символов"
                }
                required
                minLength={authTab === "register" ? 8 : undefined}
                value={formData.password}
                onChange={handleChange}
              />
              <button
                type="button"
                className="toggle-pass"
                onClick={() => setShowPassword(!showPassword)}
              >
                {showPassword ? "🙈" : "👁"}
              </button>
            </div>

            {authTab === "register" && formData.password.length > 0 && (
              <div className="strength-meter">
                {[1, 2, 3, 4].map((step) => (
                  <div
                    key={step}
                    className={`strength-step ${
                      getPasswordStrength(formData.password) >= step
                        ? "filled"
                        : ""
                    }`}
                  />
                ))}
              </div>
            )}
>>>>>>> Stashed changes
          </div>

          <button
            type="submit"
<<<<<<< Updated upstream
            disabled={isPending} // Используем флаг из TanStack Query
            className="submit-btn"
          >
            {isPending
              ? "Загрузка..."
              : authStore.authTab === "login"
=======
            disabled={loading}
            className={`submit-btn ${authTab}-btn`}
          >
            {loading
              ? "Загрузка..."
              : authTab === "login"
>>>>>>> Stashed changes
                ? "Войти"
                : "Создать аккаунт"}
          </button>
        </form>
      </div>
    </div>
  );
<<<<<<< Updated upstream
});
=======
};
>>>>>>> Stashed changes

export default AuthPage;
