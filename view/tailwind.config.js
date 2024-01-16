/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./**/**/*.{html,js,ts,go,templ}"],
  theme: {
    fontFamily: {
      sans: ['IBM Plex Sans', 'Menlo', 'monospace'],
      display: ['IBM Plex Sans', 'Menlo', 'monospace'],
      body: ['IBM Plex Sans', 'Menlo', 'monospace'],
    },
    extend: {},
  },
  plugins: [],
  variants: {
    extend: {
        display: ["group-hover"],
        opacity: ({ after }) => after(['disabled']),
        backgroundColor: ['disabled'],
    },
    width: {
        "128": "32rem",
        "192": "48rem",
    },
  },
}

