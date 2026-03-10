import React from 'react';
import { useAppRouter } from 'next/app';
import { useTailwindConfig } from 'tailwindcss/react';

const ValidatePhase1Button = () => {
  const router = useAppRouter();
  const { config } = useTailwindConfig();

  return (
    <button
      className={config.util.css({
        'bg-orange-500 hover:bg-orange-700 text-white font-bold py-2 px-4 rounded':
          true,
      })}
      onClick={() => router.push('/validate-phase-1')}
    >
      Validate Phase 1
    </button>
  );
};

export default ValidatePhase1Button;