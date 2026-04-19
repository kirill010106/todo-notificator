import { createBrowserRouter, Navigate } from "react-router-dom";
import AuthPage from "../pages/AuthPage/AuthPage";
import App from "../App";

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
        element: <div>Главная (залогинен)</div>,
      },
      {
        path: "*",
        element: <Navigate to="/" replace />,
      },
    ],
  },
]);

export default router;
