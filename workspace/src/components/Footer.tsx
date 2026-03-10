import { useCart } from '../context/CartContext';

const Footer = () => {
  const { cartCount } = useCart();

  return (
    <footer className="bg-gray-200 p-4 text-center">
      <p>&copy; 2023 My E-commerce App</p>
      <ul className="flex gap-4">
        <li>
          <a href="#">Terms and Conditions</a>
        </li>
        <li>
          <a href="#">Privacy Policy</a>
        </li>
      </ul>
      <p className="text-lg font-bold">
        You have {cartCount} items in your cart. Total: $0.00
      </p>
    </footer>
  );
};

export default Footer;