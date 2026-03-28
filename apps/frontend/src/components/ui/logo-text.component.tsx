import React from 'react';

export const LogoTextComponent = () => {
  return (
    <div className="flex items-center gap-2">
      <img src="/1hood-logo.svg" alt="1hood" width={32} height={32} />
      <span className="text-xl font-bold">1hood</span>
    </div>
  );
};
