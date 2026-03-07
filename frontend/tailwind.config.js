/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-canvas': '#2d2d3d',
        'bg-surface': '#363647',
        'bg-elevated': '#3f3f53',
        'border-subtle': '#4a4a5e',
        'text-primary': '#e1e8ed',
        'text-secondary': '#8899a6',
        accent: '#00d4ff',
        'accent-purple': '#7b2ff7',
        'status-up': '#00c853',
        'status-down': '#ff1744',
        'status-probing': '#ffc107',
        'status-unknown': '#657786',
      },
      boxShadow: {
        canvas: '0 24px 60px rgba(0, 0, 0, 0.28)',
      },
      fontFamily: {
        sans: ['IBM Plex Sans', 'Segoe UI', 'sans-serif'],
      },
    },
  },
  plugins: [],
};
