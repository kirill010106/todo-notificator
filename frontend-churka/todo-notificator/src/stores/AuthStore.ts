import { makeAutoObservable } from "mobx";
import { runInAction } from "mobx";

class AuthStore {
  authTab: "login" | "register" = "login";
  showPassword = false;
  message: { text: string; type: "success" | "error" } | null = null;
  formData = {
    email: "",
    password: "",
  };

  constructor() {
    makeAutoObservable(this);
  }

  setMessage(text: string, type: "success" | "error" = "success") {
    this.message = { text, type };
    setTimeout(() => {
      runInAction(() => {
        this.message = null;
      });
    }, 4000);
  }

  setAuthTab(tab: "login" | "register") {
    this.authTab = tab;
    this.resetForm();
  }

  setFormField(name: keyof typeof this.formData, value: string) {
    this.formData[name] = value;
  }

  togglePasswordVisibility() {
    this.showPassword = !this.showPassword;
  }

  resetForm() {
    this.formData = { email: "", password: "" };
    this.showPassword = false;
  }

  get passwordStrength() {
    const pass = this.formData.password;
    if (!pass) return 0;
    let strength = 0;
    if (pass.length >= 8) strength++;
    if (/[A-Z]/.test(pass)) strength++;
    if (/[0-9]/.test(pass)) strength++;
    return strength;
  }
}

export const authStore = new AuthStore();
