import { useCart } from '../context/CartContext';
import { Product } from '../types';
import ProductCard from '../components/ProductCard';

const Products = () => {
  const { cartCount } = useCart();
  const products = [
    { id: '1', name: 'Product 1', price: 9.99, image: 'image1.jpg', category: 'Electronics' },
    { id: '2', name: 'Product 2', price: 19.99, image: 'image2.jpg', category: 'Clothing' },
    { id: '3', name: 'Product 3', price: 29.99, image: 'image3.jpg', category: 'Home & Garden' },
    // Add more products...
  ];

  const [category, setCategory] = useState('all');

  const filteredProducts = products.filter(
    (product) => category === 'all' || product.category === category
  );

  return (
    <div className="max-w-md mx-auto p-4">
      <h1 className="text-3xl font-bold">Products</h1>
      <ul className="flex flex-wrap justify-center gap-4">
        {filteredProducts.map((product) => (
          <ProductCard key={product.id} product={product} />
        ))}
      </ul>
      <div className="flex justify-center">
        <button
          className="bg-indigo-500 p-2 rounded"
          onClick={() => setCategory('Electronics')}
        >
          Electronics
        </button>
        <button
          className="bg-indigo-500 p-2 rounded"
          onClick={() => setCategory('Clothing')}
        >
          Clothing
        </button>
        <button
          className="bg-indigo-500 p-2 rounded"
          onClick={() => setCategory('Home & Garden')}
        >
          Home & Garden
        </button>
      </div>
      <p className="text-lg font-bold">
        You have {cartCount} items in your cart. Total: $0.00
      </p>
    </div>
  );
};

export default Products;