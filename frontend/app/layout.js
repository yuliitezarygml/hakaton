import './globals.css'

export const metadata = {
  title: 'Text Analyzer',
  description: 'Анализ текстов на дезинформацию и манипуляции',
}

export default function RootLayout({ children }) {
  return (
    <html lang="ru">
      <body>{children}</body>
    </html>
  )
}
