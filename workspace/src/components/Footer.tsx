import React from 'react';

const Footer: React.FC = () => {
  return (
    <footer className="bg-gray-200 p-4 flex justify-center items-center">
      <p className="text-gray-600 text-sm">
        Copyright &copy; {new Date().getFullYear()} All Rights Reserved.
      </p>
    </footer>
  );
};

export default Footer;