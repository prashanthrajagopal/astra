import React from 'react';

interface Props {
  items: { label: string; href: string }[];
}

const Nav: React.FC<Props> = ({ items }) => {
  return (
    <nav
      className="bg-gray-200 p-4 flex justify-center items-center lg:justify-end lg:items-center"
    >
      <ul className="flex justify-center lg:justify-end">
        {items.map((item) => (
          <li key={item.href}>
            <Link href={item.href}>
              <a
                className="text-gray-600 text-lg font-bold hover:text-indigo-500 transition duration-300 ease-in-out"
                aria-label={item.label}
              >
                {item.label}
              </a>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
};

export default Nav;