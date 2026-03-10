import Link from 'next/link';

const Header: React.FC = () => {
  return (
    <header>
      <nav>
        <ul>
          <li>
            <Link href="/">
              <a>Home</a>
            </Link>
          </li>
          <li>
            <Link href="/api/validated">
              <a>Validate API</a>
            </Link>
          </li>
        </ul>
      </nav>
    </header>
  );
};

export default Header;