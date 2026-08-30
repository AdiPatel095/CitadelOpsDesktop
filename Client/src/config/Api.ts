// Environment variable access
const getEnv = (key: string, defaultValue: string = ''): string => {
    return import.meta.env[key] || defaultValue;
};

export const API_CONFIG = {
    BASE_URL: getEnv('VITE_API_BASE_URL'),
    ENDPOINTS: {},
};
