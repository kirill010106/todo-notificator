import { Outlet } from "react-router-dom";
<<<<<<< Updated upstream
import { QueryClientProvider } from "@tanstack/react-query";
import queryClient from "./queryClient/queryClient";

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Outlet />
    </QueryClientProvider>
=======

function App() {
  return (
    <>
      <Outlet />
    </>
>>>>>>> Stashed changes
  );
}

export default App;
