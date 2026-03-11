import Head from 'next/head';
import { ProductImage } from '../components/ProductImage';
import { ProductDetails } from '../components/ProductDetails';
import { useState } from 'react';

const ProductPage = () => {
  const [quantity, setQuantity] = useState(1);

  return (
    <div>
      <Head>
        <title>Product {process.env.NEXT_ID}</title>
      </Head>
      <ProductImage />
      <ProductDetails />
      <div className="flex justify-center">
        <button
          className="bg-orange-500 hover:bg-orange-700 text-white font-bold py-2 px-4 rounded"
          onClick={() => console.log(`Added ${quantity} to cart`)}
        >
          Add to cart
        </button>
        <select
          value={quantity}
          onChange={(e) => setQuantity(Number(e.target.value))}
          className="bg-gray-200 appearance-none border rounded py-2 px-4 text-gray-700 text-lg leading-tight focus:outline-none focus:border-blue-600"
        >
          {[...Array(10)].map((_, i) => (
            <option key={i} value={i + 1}>
              {i + 1}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
};

export default ProductPage;