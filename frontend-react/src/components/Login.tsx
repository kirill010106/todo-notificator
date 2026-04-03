import { useState } from 'react';
import { useAuth } from '../store/useAuth';
import { loginUser } from '../api/pomodoro';

export function Login() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const { login } = useAuth();

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const tokens = await loginUser(email, password);
      login(tokens.accessToken, tokens.refreshToken);
    } catch (e: any) {
      alert(e.response?.data?.error || "Ошибка входа");
    }
  };

  return (
    <form className="flex flex-col gap-4 p-8 bg-white shadow-xl rounded-2xl w-full max-w-sm mx-auto" onSubmit={handleLogin}>
      <h2 className="text-2xl font-bold mb-4 text-center">Вход в систему</h2>
      <input
        className="border p-3 rounded-xl focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 outline-none transition-all"
        type="email"
        placeholder="Почта"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
      />
      <input
        className="border p-3 rounded-xl focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 outline-none transition-all"
        type="password"
        placeholder="Пароль"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
      />
      <button type="submit" className="bg-indigo-600 hover:bg-indigo-700 text-white font-medium py-3 rounded-xl mt-2 transition-colors">Войти</button>
    </form>
  );
}
