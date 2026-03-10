import { useState } from 'react';
import { Rating } from '@material-ui/core';
import { useCart } from '../hooks/useCart';
import { useQuantity } from '../hooks/useQuantity';

interface ProductDetailProps {
  product: Product;
}

const ProductDetail = ({ product }: ProductDetailProps) => {
  const [quantity, setQuantity] = useState(1);
  const { addToCart } = useCart();
  const { updateQuantity } = useQuantity();

  return (
    <div className="max-w-md mx-auto p-10">
      <img src={product.image} alt={product.name} className="w-full h-64 object-cover" />
      <h2 className="text-3xl font-bold">{product.name}</h2>
      <p className="text-gray-700">{product.description}</p>
      <div className="flex justify-center my-4">
        <Rating name="read-only" value={product.rating} readOnly />
      </div>
      <p className="text-2xl font-bold">{product.price}</p>
      <p className="text-gray-700">In stock: {product.stockStatus}</p>
      <div className="flex justify-center my-4">
        <select
          value={quantity}
          onChange={e => setQuantity(parseInt(e.target.value, 10))}
          className="w-full p-2 text-lg font-bold"
        >
          {[...Array(10)].map((_, i) => (
            <option key={i} value={i + 1}>
              {i + 1}
            </option>
          ))}
        </select>
        <button
          onClick={() => addToCart(product.id, quantity)}
          className="bg-orange-500 hover:bg-orange-700 text-white font-bold py-2 px-4 rounded"
        >
          Add to Cart
        </button>
      </div>
    </div>
  );
};

export default ProductDetail;