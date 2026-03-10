src/layouts/AppLayout.tsx
import React from 'react';
import { useTheme } from 'next-themes';
import Header from '../components/Header';
import Footer from '../components/Footer';

const AppLayout: React.FC = ({ children }) => {
  const { theme } = useTheme();

  return (
    <div className={`h-screen flex flex-col ${theme === 'light' ? 'bg-white' : 'bg-gray-900'}`}>
      <Header />
      <main className="flex-1 overflow-y-auto p-4">
        {children}
      </main>
      <Footer />
    </div>
  );
};

export default AppLayout;
