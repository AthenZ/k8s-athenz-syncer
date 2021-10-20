package cr

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/yahoo/athenz/clients/go/zms"
	athenz_domain "github.com/yahoo/k8s-athenz-syncer/pkg/apis/athenz/v1"
	athenzClientset "github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned"
	athenzclient "github.com/yahoo/k8s-athenz-syncer/pkg/client/clientset/versioned/typed/athenz/v1"
	"github.com/yahoo/k8s-athenz-syncer/pkg/log"
	apiError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const trustDomainIndexKey = "trustDomain"

// CRUtil - cr resource struct
type CRUtil struct {
	athenzClientset athenzclient.AthenzV1Interface
	CrIndexInformer cache.SharedIndexInformer
}

// NewCRUtil - create new cr resource object
func NewCRUtil(athenzClientset athenzClientset.Interface, crIndexInformer cache.SharedIndexInformer) *CRUtil {
	cr := &CRUtil{
		athenzClientset: athenzClientset.AthenzV1(),
		CrIndexInformer: crIndexInformer,
	}
	return cr
}

// CreateUpdateAthenzDomain - create AthenzDomain Custom Resource with data from Athenz
func (c *CRUtil) CreateUpdateAthenzDomain(ctx context.Context, domain string, domainData *zms.SignedDomain) (cr *athenz_domain.AthenzDomain, err error) {
	if domainData == nil {
		return nil, errors.New("Domain data from ZMS API call is nil")
	}
	athenzDomainClient := c.athenzClientset.AthenzDomains()
	newCR := &athenz_domain.AthenzDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name: domain,
		},
		Spec: athenz_domain.AthenzDomainSpec{
			SignedDomain: *domainData,
		},
		Status: athenz_domain.AthenzDomainStatus{
			Message: "",
		},
	}

	obj, exist, err := c.GetCRByName(domain)
	if err != nil {
		return nil, fmt.Errorf("Did not find key in store. Error while looking up for key: %v", err)
	}
	if !exist {
		cr, err = athenzDomainClient.Create(ctx, newCR, metav1.CreateOptions{})
		if err == nil {
			return cr, nil
		} else if !apiError.IsAlreadyExists(err) {
			return nil, fmt.Errorf("Failed to create new AthenzDomain CR: %s. Error: %v", domain, err)
		}
	}
	return c.updateCR(ctx, obj, newCR)
}

// updateCR - perform an update operation on existing AthenzDomain CR
func (c *CRUtil) updateCR(ctx context.Context, object *athenz_domain.AthenzDomain, newCR *athenz_domain.AthenzDomain) (*athenz_domain.AthenzDomain, error) {
	if object == nil || newCR == nil {
		return nil, errors.New("one of the domain objects to compare is empty")
	}
	oldObjCopy := object.Spec.DeepCopy()
	newObjCopy := newCR.Spec.DeepCopy()
	oldObjCopy.Signature = ""
	oldObjCopy.Domain.Policies.Signature = ""
	newObjCopy.Signature = ""
	newObjCopy.Domain.Policies.Signature = ""
	eql := reflect.DeepEqual(oldObjCopy, newObjCopy)
	statusEql := reflect.DeepEqual(object.Status, newCR.Status)
	if eql && statusEql {
		log.Info("AthenzDomain CR is up to date, skipping CR update.")
		return nil, nil
	}
	resourceVersion := object.ResourceVersion
	newCR.ObjectMeta.ResourceVersion = resourceVersion
	return c.athenzClientset.AthenzDomains().Update(ctx, newCR, metav1.UpdateOptions{})
}

// GetCRByName - get AthenzDomain CR by domain
func (c *CRUtil) GetCRByName(domain string) (*athenz_domain.AthenzDomain, bool, error) {
	store := c.CrIndexInformer.GetStore()
	// Store.GetByKey() will always return a nil for error field
	object, exist, _ := store.GetByKey(domain)
	if !exist {
		return nil, exist, nil
	}
	obj, ok := object.(*athenz_domain.AthenzDomain)
	if !ok {
		return nil, exist, fmt.Errorf("Error occurred when casting AthenzDomain object")
	}
	return obj, exist, nil
}

// RemoveAthenzDomain - delete AthenzDomain CR from Cluster
func (c *CRUtil) RemoveAthenzDomain(ctx context.Context, domain string) error {
	obj, exist, err := c.GetCRByName(domain)
	if err != nil {
		log.Infof("Error occurred in getCRByName function. Error: %v", err)
		return err
	}
	if exist && obj != nil {
		err := c.athenzClientset.AthenzDomains().Delete(ctx, domain, metav1.DeleteOptions{})
		if err != nil {
			log.Error("Error occurred when deleting AthenzDomain Custom Resource in the Cluster")
			return err
		}
		log.Info("Deleted invalid AthenzDomain file")
	}
	return nil
}

// GetLatestTimestamp - get the latest etag from all AthenzDomain CRs in the store (used initially)
func (c *CRUtil) GetLatestTimestamp() string {
	crs := c.CrIndexInformer.GetStore().List()
	// initial date to compare with etags from CRs
	latest := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	if len(crs) == 0 {
		return ""
	}
	for _, domain := range crs {
		cr, ok := domain.(*athenz_domain.AthenzDomain)
		if !ok {
			return ""
		}
		timestamp := cr.Spec.Domain.Modified
		if timestamp.After(latest) {
			latest = timestamp.Time
		}
	}
	timestr := latest.Format(time.RFC3339)
	if timestr == "1970-01-01T00:00:00Z" {
		return ""
	}
	return timestr
}

// IsTrustDomain looks up the trust domain index to determine if the given domain serves as a trust for a delegated role
func (c *CRUtil) IsTrustDomain(domainName string) bool {
	delegatedList, err := c.CrIndexInformer.GetIndexer().ByIndex(trustDomainIndexKey, domainName)
	if err != nil {
		log.Errorf("Error while looking up the trust domain indexer: %s", err)
		return false
	}
	if len(delegatedList) == 0 {
		return false
	}
	return true
}

// TrustDomainIndexFunc returns the list of trust domains as defined by the delegated roles in an Athenz domain
func TrustDomainIndexFunc(obj interface{}) ([]string, error) {
	domain, ok := obj.(*athenz_domain.AthenzDomain)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("Couldn't get object from tombstone %#v", obj)
			return []string{}, nil
		}
		domain, ok = tombstone.Obj.(*athenz_domain.AthenzDomain)
		if !ok {
			log.Errorf("Tombstone contained object that is not an Athenz Domain %#v", obj)
			return []string{}, nil
		}
	}
	data := domain.Spec.Domain
	if data == nil {
		return []string{}, nil
	}
	trustDomains := make([]string, 0)
	for _, role := range domain.Spec.Domain.Roles {
		if role.Trust != "" {
			trustDomains = append(trustDomains, string(role.Trust))
		}
	}
	return trustDomains, nil
}

// UpdateErrorStatus - add error status field in CR when zms call returns error
func (c *CRUtil) UpdateErrorStatus(ctx context.Context, obj *athenz_domain.AthenzDomain) {
	_, err := c.athenzClientset.AthenzDomains().Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err)
	}
}
