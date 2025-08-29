
export const ALLOWED_PHONE_NUMBERS = {
  andres: "573192314936",
  harold: "573045360175",
  juan: "573229316570"
};

export const isPhoneNumberAllowed =(phoneNumber: string): boolean  => {
  const allowedNumbers = Object.values(ALLOWED_PHONE_NUMBERS);
  return allowedNumbers.includes(phoneNumber);
}