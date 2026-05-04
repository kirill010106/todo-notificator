import { createBrowserRouter, Navigate } from "react-router-dom";
import AuthPage from "../pages/AuthPage/AuthPage";
import App from "../App";
<<<<<<< Updated upstream
import MainPage from "../pages/MainPage/MainPage";
=======

// Создаём компонент для защищённых маршрутов
const ProtectedRoute = ({ children }) => {
  const token = localStorage.getItem("token");

  if (!token) {
    return <Navigate to="/auth" replace />;
  }

  return children;
};
>>>>>>> Stashed changes

const router = createBrowserRouter([
  {
    path: "/",
    element: <App />,
    children: [
      {
        index: true,
        element: <AuthPage />,
      },
      {
        path: "auth",
        element: <AuthPage />,
      },
      {
        path: "main",
<<<<<<< Updated upstream
        element: <MainPage />,
=======
        element: (
          <ProtectedRoute>
            <div>Главная (залогинен)</div>
          </ProtectedRoute>
        ),
>>>>>>> Stashed changes
      },
      {
        path: "*",
        element: <Navigate to="/" replace />,
      },
    ],
  },
]);

export default router;
