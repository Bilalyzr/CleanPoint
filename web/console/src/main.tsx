import React from "react";
import ReactDOM from "react-dom/client";
import { createHashRouter, RouterProvider } from "react-router-dom";
import App from "./App";
import Login from "./pages/Login";
import Overview from "./pages/Overview";
import Workloads from "./pages/Workloads";
import WorkloadDetail from "./pages/WorkloadDetail";
import Simulation from "./pages/Simulation";
import Evaluations from "./pages/Evaluations";
import Audit from "./pages/Audit";
import Settings from "./pages/Settings";
import "./styles.css";

const router = createHashRouter([
  { path: "/login", element: <Login /> },
  {
    path: "/",
    element: <App />,
    children: [
      { index: true, element: <Overview /> },
      { path: "workloads", element: <Workloads /> },
      { path: "workload/:id/:tab?", element: <WorkloadDetail /> },
      { path: "simulation", element: <Simulation /> },
      { path: "evaluations", element: <Evaluations /> },
      { path: "audit", element: <Audit /> },
      { path: "settings", element: <Settings /> },
    ],
  },
]);

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>
);
