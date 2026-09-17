package clients

import (
	"diana-contabilitate/backend/internal/accounting"
	"diana-contabilitate/backend/internal/accountingdate"
	"diana-contabilitate/backend/internal/spv"
)

type Section struct {
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
	NextAction  string `json:"nextAction"`
	Target      string `json:"target"`
}
type Readiness struct {
	Company                Section  `json:"company"`
	AccountingProfile      Section  `json:"accountingProfile"`
	ANAF                   Section  `json:"anaf"`
	Saga                   Section  `json:"saga"`
	Classification         Section  `json:"classification"`
	Overall                string   `json:"overall"`
	Blockers               []string `json:"blockers"`
	CurrentProfileID       string   `json:"currentProfileId"`
	SagaConfigurationReady bool     `json:"sagaConfigurationReady"`
	SagaMappingApproved    bool     `json:"sagaMappingApproved"`
	SagaValidation         string   `json:"sagaValidation"`
}

func section(status, explanation, action, target string) Section {
	return Section{status, explanation, action, target}
}

// Derive never persists a completion flag and never treats TEST_ONLY releases as production.
func Derive(c Client, profiles []*accounting.Profile, packs []*accounting.Pack, enabled bool, anaf spv.ConnectionView, date accountingdate.Date) Readiness {
	r := Readiness{Blockers: []string{}, SagaValidation: "VALIDATION_PENDING"}
	r.Company = section("READY", "Identitatea companiei este salvată.", "Editează datele companiei", "company")
	if c.Name == "" || c.CUI == "" {
		r.Company = section("INCOMPLETE", "Identitatea companiei este incompletă.", "Completează datele companiei", "company")
	}
	r.AccountingProfile = section("NOT_STARTED", "Nu există profil contabil și fiscal.", "Configurează profilul fiscal", "accounting-profile")
	var current *accounting.Profile
	applicable := 0
	var approved *accounting.Profile
	for _, p := range profiles {
		if p == nil {
			continue
		}
		if date.Within(p.EffectiveFrom, p.EffectiveTo) {
			if current == nil || p.Version > current.Version {
				current = p
			}
			if !p.TestOnly && p.Valid(c.ID, date) {
				applicable++
				approved = p
			}
		}
	}
	if applicable == 1 {
		current = approved
	}
	if len(profiles) > 0 && current == nil {
		r.AccountingProfile = section("INCOMPLETE", "Există numai profiluri istorice sau viitoare.", "Configurează un profil aplicabil astăzi", "accounting-profile")
	}
	if current != nil {
		r.CurrentProfileID = current.ID
		r.AccountingProfile = section("INCOMPLETE", "Profil configurat; aprobarea cu dovezi este necesară.", "Salvează o versiune aprobată", "accounting-profile")
		if !current.TestOnly && current.Valid(c.ID, date) && applicable == 1 {
			r.AccountingProfile = section("READY", "Profil aplicabil aprobat; faptele UNKNOWN rămân necunoscute.", "Vezi versiunile profilului", "accounting-profile")
		}
		if applicable > 1 {
			r.AccountingProfile = section("ACTION_REQUIRED", "Mai multe profiluri aprobate se suprapun; clasificarea va cere revizuire.", "Revizuiește perioadele profilurilor", "accounting-profile")
		}
	}
	r.ANAF = section("NOT_STARTED", "Autorizare ANAF cu certificat digital în browser/token USB. Diana nu stochează certificatul sau PIN-ul.", "Pregătește certificatul și autorizează ANAF", "anaf-spv")
	if !anaf.ConfigurationReady {
		r.ANAF = section("ACTION_REQUIRED", "Configurația aplicației ANAF nu este disponibilă în acest mediu.", "Solicită configurarea aplicației ANAF", "anaf-spv")
	} else {
		switch anaf.Status {
		case spv.StatusConnected:
			r.ANAF = section("READY", "Autorizat; acoperirea CUI nu este confirmată de OAuth. Fiecare factură este verificată la import.", "Sincronizează acum", "anaf-spv")
			if anaf.SafeErrorCode != "" {
				r.ANAF = section("ACTION_REQUIRED", "Ultima sincronizare a eșuat.", "Verifică sincronizarea ANAF", "anaf-spv")
			}
		case spv.StatusNeedsReauthentication, spv.StatusError:
			r.ANAF = section("ACTION_REQUIRED", "Accesul ANAF necesită verificare sau reautorizare.", "Reautorizează ANAF", "anaf-spv")
		case spv.StatusDisabled:
			r.ANAF = section("INCOMPLETE", "Conexiunea este dezactivată; istoricul este păstrat.", "Reautorizează ANAF", "anaf-spv")
		}
	}
	if anaf.SafeErrorCode == "CLIENT_INACTIVE" {
		r.ANAF = section("INCOMPLETE", "Client inactiv: operațiile ANAF noi sunt oprite.", "Reactivează clientul", "client-lifecycle")
	}
	r.Saga = section("NOT_STARTED", "Import manual de fișier XML în SAGA C.", "Configurează exportul SAGA", "saga-setup")
	r.SagaConfigurationReady = enabled && r.Company.Status == "READY"
	if enabled {
		r.Saga = section("INCOMPLETE", "Export solicitat; identitatea companiei este incompletă.", "Completează datele companiei", "saga-setup")
	}
	if r.SagaConfigurationReady {
		r.Saga = section("READY", "Export SAGA configurat. Validarea în SAGA este în așteptare; eligibilitatea fiecărei facturi se verifică separat.", "Vezi configurarea și validarea mapării", "saga-setup")
	}
	r.Classification = section("NOT_STARTED", "Reguli de producție neconfigurate. Facturile pot fi importate și trimise la revizuire.", "Regulile automate nu sunt încă aprobate", "classification-status")
	validPacks := 0
	for _, p := range packs {
		if p != nil && !p.TestOnly && !p.Mapping.TestOnly && current != nil && !current.TestOnly && applicable == 1 && p.Valid(current, c.ID, date, false) && len(p.Rules) > 0 {
			validPacks++
			if p.Mapping.Approved && p.Mapping.Approval.Valid() && p.Mapping.OrdinaryFullOmission {
				r.SagaMappingApproved = true
			}
		}
	}
	if validPacks == 1 && r.AccountingProfile.Status == "READY" {
		r.Classification = section("READY", "Pack de producție aplicabil aprobat.", "Vezi regulile", "classification-status")
	} else if validPacks > 1 {
		r.Classification = section("ACTION_REQUIRED", "Pack-uri de producție suprapuse.", "Revizuiește release-urile", "classification-status")
	}
	r.Overall = "CONFIGURATION_INCOMPLETE"
	if r.Company.Status == "READY" && r.AccountingProfile.Status == "READY" && r.ANAF.Status == "READY" && r.Saga.Status == "READY" {
		r.Overall = "CORE_CONFIGURED_AUTOMATION_PENDING"
		if r.Classification.Status == "READY" {
			r.Overall = "CORE_AND_AUTOMATION_CONFIGURED_VALIDATION_PENDING"
		}
	}
	for _, s := range []Section{r.Company, r.AccountingProfile, r.ANAF, r.Saga, r.Classification} {
		if s.Status != "READY" {
			r.Blockers = append(r.Blockers, s.Explanation)
		}
	}
	if c.Status == Inactive {
		r.Overall = "INACTIVE"
		r.Blockers = append(r.Blockers, "Client inactiv: sincronizările noi sunt oprite; documentele deja înregistrate se finalizează.")
	}
	return r
}

func (r *Readiness) ApplyANAF(c Client, view spv.ConnectionView) {
	derived := Derive(c, nil, nil, false, view, "")
	old := r.ANAF.Explanation
	r.ANAF = derived.ANAF
	blockers := []string{}
	for _, b := range r.Blockers {
		if b != old {
			blockers = append(blockers, b)
		}
	}
	if r.ANAF.Status != "READY" {
		blockers = append(blockers, r.ANAF.Explanation)
	}
	r.Blockers = blockers
	if c.Status == Inactive {
		r.Overall = "INACTIVE"
		return
	}
	r.Overall = "CONFIGURATION_INCOMPLETE"
	if r.Company.Status == "READY" && r.AccountingProfile.Status == "READY" && r.ANAF.Status == "READY" && r.Saga.Status == "READY" {
		r.Overall = "CORE_CONFIGURED_AUTOMATION_PENDING"
		if r.Classification.Status == "READY" {
			r.Overall = "CORE_AND_AUTOMATION_CONFIGURED_VALIDATION_PENDING"
		}
	}
}
