import React from 'react';
import './TSPLayout.css';

interface TSPLayoutProps {
  children: React.ReactNode;
}

const TSPLayout: React.FC<TSPLayoutProps> = ({ children }) => {
  return (
    <div className="tsp-layout">
      {children}
    </div>
  );
};

export default TSPLayout;