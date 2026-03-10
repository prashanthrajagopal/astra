import { useCart } from '../context/CartContext';
import Link from 'next/link';

const Header = () => {
  const { cartCount } = useCart();

  return (
    <header className="bg-indigo-500 p-4">
      <nav className="flex justify-between items-center">
        <Link href="/" passHref>
          <a className="text-white font-bold">Logo</a>
        </Link>
        <ul className="flex gap-4">
          <li>
            <Link href="/products" passHref>
              <a>Products</a>
            </Link>
          </li>
          <li>
            <Link href="/cart" passHref>
              <a>Cart ({cartCount})</a>
            </Link>
          </li>
        </ul>
        <div className="flex items-center">
          <input
            type="search"
            placeholder="Search"
            className="bg-gray-200 p-2 w-full"
          />
          <button className="bg-indigo-500 p-2">Search</button>
        </div>
      </nav>
    </header>
  );
};

export default Header;