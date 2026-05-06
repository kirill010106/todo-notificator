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
  };

  return (
    <div className="auth-container">
      <div className="auth-card">
        <header className="auth-header">
          <div className="auth-logo">✅</div>
          <h1>ToDo Notificator</h1>
        </header>

        <nav className="auth-tabs">
          {(["login", "register"] as const).map((tab) => (
            <button
              key={tab}
              className={`tab-btn ${authStore.authTab === tab ? "active" : ""}`}
              onClick={() => authStore.setAuthTab(tab)}
            >
              {tab === "login" ? "Войти" : "Регистрация"}
            </button>
          ))}
        </nav>

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
            />
          </div>

          <div className="input-group">
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
          </div>

          <button
            type="submit"
            disabled={isPending} // Используем флаг из TanStack Query
            className="submit-btn"
          >
            {isPending
              ? "Загрузка..."
              : authStore.authTab === "login"
                ? "Войти"
                : "Создать аккаунт"}
          </button>
        </form>
      </div>
    </div>
  );
});

export default AuthPage;
