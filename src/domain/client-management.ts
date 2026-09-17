import type { Client } from './invoice'
export type ClientLifecycle = 'ONBOARDING' | 'ACTIVE' | 'INACTIVE'
export interface Company { name:string;cui:string;displayName:string;registrationNumber:string;country:string;address:string;city:string;region:string;postalCode:string;email:string;phone:string;defaultCurrency:string }
export interface ClientProfile {id?:string;clientId?:string;version?:number;testOnly?:boolean;effectiveFrom:string;effectiveTo?:string;framework:string;taxRegime:string;vatRegistration:string;deductionActivity:string;cashAccounting:string;proRata:string;chartPolicy:string;accountCodes:string[];approval?:{actor:string;at:string;evidence:string[]} }
export interface ReadinessSection {status:'NOT_STARTED'|'INCOMPLETE'|'READY'|'ACTION_REQUIRED';explanation:string;nextAction:string;target:string}
export interface ClientReadiness {company:ReadinessSection;accountingProfile:ReadinessSection;anaf:ReadinessSection;saga:ReadinessSection;classification:ReadinessSection;overall:string;blockers:string[];currentProfileId?:string;sagaConfigurationReady:boolean;sagaMappingApproved:boolean;sagaValidation:string}
export interface ClientDetail {client:Client;profiles:ClientProfile[];sagaEnabled:boolean;history:Array<{id:string;label:string;actor:string;timestamp:string;detail:string}>;onboarding:ClientReadiness}
export type ClientWrite =
 | {kind:'create';company:Company}
 | {kind:'company';clientId:string;company:Company;expectedRevision:number}
 | {kind:'lifecycle';clientId:string;status:ClientLifecycle;expectedRevision:number}
 | {kind:'accounting-profiles';clientId:string;profile:ClientProfile;approve:boolean;evidence:string[];expectedProfileVersion:number;expectedRevision:number}
 | {kind:'saga-configuration';clientId:string;sagaEnabled:boolean;expectedRevision:number}
export const emptyCompany:Company={name:'',cui:'',displayName:'',registrationNumber:'',country:'RO',address:'',city:'',region:'',postalCode:'',email:'',phone:'',defaultCurrency:'RON'}
export const lifecycleLabels:Record<ClientLifecycle,string>={ONBOARDING:'În configurare',ACTIVE:'Activ',INACTIVE:'Inactiv'}
export const sectionLabels={NOT_STARTED:'Neconfigurat',INCOMPLETE:'Incomplet',READY:'Configurat',ACTION_REQUIRED:'Acțiune necesară'}
