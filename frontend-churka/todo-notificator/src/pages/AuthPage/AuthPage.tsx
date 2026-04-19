import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import "./AuthPage.css";

const API_BASE = "http://localhost:8082/api/v1";

type AuthTab = "login" | "register";

const AuthPage: React.FC = () => {
  const navigate = useNavigate();
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

    const path = authTab === "login" ? "/login" : "/register";

    try {
      const response = await fetch(`${API_BASE}${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(formData),
      });

      const data = await response.json();

      if (!response.ok) {
        // Обработка ошибок по твоему Swagger
        if (response.status === 401)
          throw new Error("Неверный логин или пароль");
        if (response.status === 409)
          throw new Error("Этот email уже зарегистрирован");
        throw new Error(data.error || "Ошибка сервера");
      }

      if (authTab === "login") {
        // Сохраняем токены (snake_case из Swagger)
        if (data.access_token) {
          localStorage.setItem("token", data.access_token);
          localStorage.setItem("refresh", data.refresh_token);

          // Используем navigate вместо window.location для SPA-перехода
          navigate("/main");
        }
      } else {
        // Успешная регистрация
        alert(`Регистрация успешна! (ID: ${data.user_id})`);
        setAuthTab("login");
        setFormData((prev) => ({ ...prev, password: "" }));
      }
    } catch (error: any) {
      alert(`Ошибка: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-container">
      <div className="auth-card">
        <header className="auth-header">
          <div className="auth-logo">✅</div>
          <h1>ToDo Notificator</h1>
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
            >
              {tab === "login" ? "Войти" : "Регистрация"}
            </button>
          ))}
        </nav>

        <form onSubmit={handleSubmit} className="auth-form">
          <div className="input-group">
            <label htmlFor="email">Email</label>
            <input
              id="email"
              name="email"
              type="email"
              placeholder="you@example.com"
              autoComplete="email"
              required
              value={formData.email}
              onChange={handleChange}
            />
          </div>

          <div className="input-group">
            <label htmlFor="password">Пароль</label>
            <div className="password-wrapper">
              <input
                id="password"
                name="password"
                type={showPassword ? "text" : "password"}
                autoComplete={
                  authTab === "login" ? "current-password" : "new-password"
                }
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
                tabIndex={-1}
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
          </div>

          <button
            type="submit"
            disabled={loading}
            className={`submit-btn ${authTab}-btn`}
          >
            {loading
              ? "Загрузка..."
              : authTab === "login"
                ? "Войти"
                : "Создать аккаунт"}
          </button>
        </form>
      </div>
    </div>
  );
};

export default AuthPage;
