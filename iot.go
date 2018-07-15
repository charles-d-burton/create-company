package main

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/iot"
)

type IAMIotPolicy struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

type Statement struct {
	Effect   string   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

func (user *User) createIoTCertificate(sess *session.Session) error {
	log.Println("Generating Keys")
	iotsvc := iot.New(sess)
	//Create certificates
	keys, err := iotsvc.CreateKeysAndCertificate(&iot.CreateKeysAndCertificateInput{
		SetAsActive: aws.Bool(true),
	})
	if err != nil {
		return err
	}
	certificate := Certificate{
		CompanyID:      &user.CompanyID,
		CreatedBy:      &user.Sub,
		CertificateArn: keys.CertificateArn,
		CertificateId:  keys.CertificateId,
		CertificatePem: keys.CertificatePem,
		PrivateKey:     keys.KeyPair.PrivateKey,
		PublicKey:      keys.KeyPair.PublicKey,
	}

	//Store certs in dynamo for the company
	dynamosvc := dynamodb.New(sess)
	av, err := dynamodbattribute.MarshalMap(certificate)
	if err != nil {
		return err
	}
	_, err = dynamosvc.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("certificates"),
		Item:      av,
	})

	//Generate the IOT policy for that thing and certificate
	policy, err := user.creatIotPolicy(sess)
	if err != nil {
		return err
	}
	presp, err := iotsvc.CreatePolicy(&iot.CreatePolicyInput{
		PolicyDocument: policy,
		PolicyName:     aws.String(strings.Split(user.CompanyID, "-")[0]),
	})
	if err != nil {
		return err
	}
	// Attach policy to certificate
	_, err = iotsvc.AttachPrincipalPolicy(&iot.AttachPrincipalPolicyInput{
		PolicyName: presp.PolicyName,
		Principal:  keys.CertificateArn,
	})
	// Attach thing to certificate
	thing, err := user.createThing(iotsvc)
	_, err = iotsvc.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
		ThingName: thing.ThingName,
		Principal: keys.CertificateArn,
	})
	return err
}

//Create a thing for the company
func (user *User) createThing(iotsvc *iot.IoT) (*iot.CreateThingOutput, error) {
	log.Println("Creating Thing")
	thing, err := iotsvc.CreateThing(
		&iot.CreateThingInput{
			AttributePayload: &iot.AttributePayload{
				Attributes: map[string]*string{"company_id": aws.String(user.CompanyID)},
			},
			ThingName: aws.String(user.CompanyID),
		})
	if err != nil {
		log.Println("Failed to create thing: ", err)
		return nil, err
	}
	log.Println("Thing created", *thing.ThingArn)
	return thing, nil
}

// Generate the IOT policy document allowing only connection to the company topic
func (user *User) creatIotPolicy(sess *session.Session) (*string, error) {
	region := *sess.Config.Region
	var topic Statement
	topic.Effect = "Allow"
	topic.Action = []string{"iot:Publish", "iot:Receive", "iot:Subscribe"}
	topic.Resource = []string{"arn:aws:iot:" + region + ":" + accountID + ":" + "topic/rsmachiner/" + user.CompanyID + "/*"}

	var connect Statement
	connect.Effect = "Allow"
	connect.Action = []string{"iot:Connect"}
	connect.Resource = []string{"*"}

	statements := []Statement{topic, connect}

	var policy IAMIotPolicy
	policy.Version = "2012-10-17"
	policy.Statement = statements

	b, err := json.Marshal(policy)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	policyString := string(b)
	log.Println(policyString)
	return &policyString, nil
}
