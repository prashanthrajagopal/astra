import Link from 'next/link';

const Header = () => {
  return (
    <header className="bg-gray-800 p-4 shadow-md">
      <nav className="flex justify-between">
        <ul className="flex items-center">
          <li className="mr-4">
            <Link href="/">
              <a className="text-white hover:text-gray-300">Home</a>
            </Link>
          </li>
          <li className="mr-4">
            <Link href="/about">
              <a className="text-white hover:text-gray-300">About</a>
            </Link>
          </li>
        </ul>
      </nav>
    </header>
  );
};

export default Header;