/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Near-black canvas, dark grey cards, one blue accent. Kept as tokens so
        // a future theme change is a single edit.
        canvas: '#0b0d10',
        surface: '#14171c',
        'surface-raised': '#1b1f26',
        'surface-hover': '#222833',
        line: '#262c36',
        accent: {
          DEFAULT: '#2f7cf6',
          strong: '#1f66d6',
          soft: '#1c2c47',
        },
        ink: {
          DEFAULT: '#f2f4f7',
          muted: '#9aa4b2',
          faint: '#6b7482',
        },
        success: '#33c27f',
        warning: '#e0a83c',
        danger: '#e5544b',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'Segoe UI', 'Roboto', 'sans-serif'],
      },
      borderRadius: {
        card: '1rem',
      },
      spacing: {
        // Comfortable thumb targets; 44px is the documented minimum.
        tap: '2.75rem',
      },
      boxShadow: {
        sheet: '0 -12px 40px rgba(0, 0, 0, 0.55)',
      },
      screens: {
        xs: '400px',
      },
    },
  },
  plugins: [],
}
