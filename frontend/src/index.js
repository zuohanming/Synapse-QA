import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App.js";
import { AuthProvider } from "./context/AuthContext.js";
import "./styles/theme-blue.css";

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <AuthProvider>
      <App />
    </AuthProvider>
  </React.StrictMode>
);
