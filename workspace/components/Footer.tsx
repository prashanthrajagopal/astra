import { useState } from 'react';

const Footer = () => {
  const [copyrightYear, setCopyrightYear] = useState(new Date().getFullYear());

  return (
    <footer className="bg-gray-200 p-4">
      <p>&copy; {copyrightYear} All rights reserved.</p>
      <ul className="flex justify-center">
        <li>
          <Link href="/terms-of-service">
            <a>Terms of Service</a>
          </Link>
        </li>
        <li>
          <Link href="/privacy-policy">
            <a>Privacy Policy</a>
          </Link>
        </li>
      </ul>
    </footer>
  );
};

export default Footer;