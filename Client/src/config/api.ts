// Environment variable access
const getEnv = (key: string, defaultValue: string = ''): string => {
    return import.meta.env[key] || defaultValue;
};

export const API_CONFIG = {
    BASE_URL: getEnv('VITE_API_BASE_URL', 'https://citadelops.app/api'),
    ENDPOINTS: {
        LICENSE: {
            GET_BY_HARDWARE_ID: '/license/unverified', // GET /license/unverified/:hardwareID
        },
        EQUIPMENT: {
            RECONFIGURE_LOADOUT: '/equipment/reconfigure', // POST /equipment/reconfigure
        },
    },
};
