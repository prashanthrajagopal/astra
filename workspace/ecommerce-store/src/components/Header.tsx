import Link from 'next/link';
import { useState } from 'react';
import clsx from 'clsx';

interface Props {
  itemCount: number;
}

const Header: React.FC<Props> = ({ itemCount }) => {
  return (
    <header className="bg-white shadow-md sticky top-0 z-10">
      <nav className="max-w-7xl mx-auto p-4 flex justify-between items-center">
        <Link href="/">
          <a className="flex items-center">
            <img
              src="/logo.png"
              alt="Logo"
              width={48}
              height={48}
              className="mr-2"
            />
            <span className="font-bold text-lg">My App</span>
          </a>
        </Link>
        <ul className="flex items-center justify-end">
          <li className="mr-4">
            <Link href="/products">
              <a>Products</a>
            </Link>
          </li>
          <li className="mr-4">
            <Link href="/cart">
              <a>Cart ({itemCount})</a>
            </Link>
          </li>
          <li>
            <input
              type="search"
              placeholder="Search"
              className="w-full p-2 text-sm text-gray-700"
            />
          </li>
        </ul>
      </nav>
    </header>
  );
};

export default Header;