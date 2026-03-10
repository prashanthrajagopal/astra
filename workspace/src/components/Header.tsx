import React from 'react';
import Link from 'next/link';

interface Props {
  title: string;
}

const Header: React.FC<Props> = ({ title }) => {
  return (
    <header className="bg-indigo-500 p-4 flex justify-between items-center">
      <Link href="/">
        <a className="text-gray-200 text-lg font-bold">{title}</a>
      </Link>
      <nav className="flex justify-end">
        <ul className="flex">
          <li>
            <Link href="/about">
              <a
                className="text-gray-200 text-lg font-bold hover:text-white transition duration-300 ease-in-out"
                aria-label="About"
              >
                About
              </a>
            </Link>
          </li>
        </ul>
      </nav>
    </header>
  );
};

export default Header;