/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ["Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["ui-monospace", "Consolas", "monospace"],
      },
      colors: {
        surface: {
          DEFAULT: "#131313",
          dim: "#131313",
          bright: "#3a3939",
          container: "#201f1f",
          "container-high": "#2a2a2a",
          "container-highest": "#353534",
          "container-low": "#1c1b1b",
          "container-lowest": "#0e0e0e",
          variant: "#353534",
        },
        on: {
          surface: "#e5e2e1",
          "surface-variant": "#c6c5d8",
        },
        outline: {
          DEFAULT: "#908fa1",
          variant: "#454555",
        },
        accent: {
          DEFAULT: "#bec2ff",
          dim: "#212aca",
          container: "#212aca",
        },
        err: {
          DEFAULT: "#ffb4ab",
          container: "#93000a",
        },
      },
      borderRadius: {
        sm: "0.25rem",
      },
    },
  },
  plugins: [],
};
