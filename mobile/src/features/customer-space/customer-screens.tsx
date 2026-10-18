/**
 * Customer space (`musteri`): customer and fleet accounts signed in with a
 * phone code. Screens call the portal routes (`/v1/portal/*`), which the API
 * client sends to the mobile portal mirror. Route files re-export from here.
 */
export { CustomerClaimsScreen } from './claim-screens';
export { CustomerContractsScreen } from './contract-screens';
export { CustomerServiceReviewScreen } from './review-screens';
export { CustomerAppointmentsScreen } from './appointment-screens';
export { CustomerHomeScreen } from './home-screen';
export { CustomerProfileScreen } from './profile-screen';
export { CustomerServiceDetailScreen, CustomerServicesScreen } from './service-screens';
export { CustomerVehicleDetailScreen, CustomerVehiclesScreen } from './vehicle-screens';
export { CustomerWarrantiesScreen, CustomerWarrantyDetailScreen } from './warranty-screens';
