export const Phase = {
  Created: "created",
  Analyzing: "analyzing",
  Initializing: "initializing",
  Preprocessing: "preprocessing",
  Encrypting: "encrypting",
  Decrypting: "decrypting",
  Packing: "packing",
  Verifying: "verifying",
  Completed: "completed",
} as const;

export type Phase = (typeof Phase)[keyof typeof Phase];
