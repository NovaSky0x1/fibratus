/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        fibratus: {
          50: '#eef5ff',
          100: '#d8e8ff',
          200: '#b9d4ff',
          300: '#8ab8ff',
          400: '#60cefd',
          500: '#5894ce',
          600: '#4c6ef5',
          700: '#4263eb',
          800: '#3b5bdb',
          900: '#1a365d',
          950: '#0f1f3d',
        },
      },
      fontFamily: {
        sans: ['Jost', 'Source Sans Pro', 'system-ui', 'sans-serif'],
        mono: ['Nova Mono', 'Source Code Pro', 'ui-monospace', 'monospace'],
      },
    },
  },
  plugins: [],
}
