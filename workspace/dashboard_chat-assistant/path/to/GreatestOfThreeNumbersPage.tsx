import React from 'react';
import styles from './GreatestOfThreeNumbersPage.module.css';

interface Props {
  num1: number;
  num2: number;
  num3: number;
}

const GreatestOfThreeNumbersPage: React.FC<Props> = ({ num1, num2, num3 }) => {
  const greatestNumber = Math.max(num1, num2, num3);
  return (
    <div className={styles.container}>
      <h1>Greatest of Three Numbers</h1>
      <p>The greatest number is: {greatestNumber}</p>
    </div>
  );
};

export default GreatestOfThreeNumbersPage;