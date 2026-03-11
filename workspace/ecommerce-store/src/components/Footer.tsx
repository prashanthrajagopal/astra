import Link from 'next/link';

const Footer: React.FC = () => {
  return (
    <footer className="bg-gray-200 p-4 text-center">
      <p>&copy; 2023 My App</p>
      <ul className="flex justify-center">
        <li>
          <Link href="/terms">
            <a>Terms & Conditions</a>
          </Link>
        </li>
        <li>
          <Link href="/privacy">
            <a>Privacy Policy</a>
          </Link>
        </li>
      </ul>
    </footer>
  );
};

export default Footer;