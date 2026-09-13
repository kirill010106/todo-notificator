import { createBrowserRouter, Navigate } from "react-router-dom";
import AuthPage from "../pages/AuthPage/AuthPage";
import App from "../App";
import MainPage from "../pages/MainPage/MainPage";

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
        element: <MainPage />,
      },
      {
        path: "*",
        element: <Navigate to="/" replace />,
      },
    ],
  },
]);

export default router;
