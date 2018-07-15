package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambdacontext"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/iot"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/mitchellh/mapstructure"
	uuid "github.com/satori/go.uuid"

	"github.com/aws/aws-lambda-go/lambda"
)

var (
	accountID string
)

//Event The struct representation of the input from Cognito when a user is confirmed
type Event struct {
	Version       string `json:"version"`
	TriggerSource string `json:"triggerSource"`
	Region        string `json:"region"`
	UserPoolID    string `json:"userPoolId"`
	UserName      string `json:"userName"`
	CallerContext struct {
		ClientID string `json:"clientId"`
	} `json:"callerContext"`
	Request struct {
		UserAttributes struct {
			Email         string `json:"email"`
			GivenName     string `json:"given_name"`
			Sub           string `json:"sub"`
			EmailVerified bool   `json:"email_verified"`
		} `json:"userAttributes"`
	} `json:"request"`
}

//User the representation of a user to put into DynamoDB
type User struct {
	Email       string `json:"email"`
	Sub         string `json:"sub"`
	CompanyID   string `json:"company_id"`
	UserName    string `json:"user_name"`
	Payed       bool   `json:"payed"`
	ServiceTier int    `json:"service_tier"`
	Role        string `json:"role"`
}

//Creater the IoT certificate for the company to be stored in dynamo
type Certificate struct {
	CompanyID      *string `json:"company_id"`
	CreatedBy      *string `json:"sub"`
	CertificateArn *string `json:"certificate_arn"`
	CertificateId  *string `json:"certificate_id"`
	CertificatePem *string `json:"certificate_pem"`
	PrivateKey     *string `json:"private_key"`
	PublicKey      *string `json:"public_key"`
}

//Create a thing for the company
type RSThing struct {
	CompanyID *string                `json:"company_id"`
	CreatedBy *string                `json:"sub"`
	Thing     *iot.CreateThingOutput `json:"thing"`
}

//HandleRequest handles the request from Cognito
func HandleRequest(ctx context.Context, event interface{}) (interface{}, error) {
	lc, _ := lambdacontext.FromContext(ctx)
	accountID = strings.Split(lc.InvokedFunctionArn, ":")[4]
	var eventStruct Event
	err := mapstructure.Decode(event, &eventStruct)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println("SUB: ", eventStruct.Request.UserAttributes.Sub)
	log.Println(event)
	uuid := uuid.Must(uuid.NewV4()).String()
	var user User
	user.Sub = eventStruct.Request.UserAttributes.Sub
	user.CompanyID = uuid
	user.Email = eventStruct.Request.UserAttributes.Email
	user.UserName = eventStruct.UserName
	user.Payed = false
	user.ServiceTier = 0
	user.Role = "admin"
	sess, err := session.NewSession()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	err = user.createUserRecord(sess)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = user.createIoTCertificate(sess)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	err = user.creatS3Bucket(sess)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return event, err
}

func (user *User) createUserRecord(sess *session.Session) error {
	log.Println("Creating Dynamo Entry")
	svc := dynamodb.New(sess)
	av, err := dynamodbattribute.MarshalMap(user)
	if err != nil {
		return err
	}
	_, err = svc.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("users"),
		Item:      av,
	})
	return err
}

func (user *User) creatS3Bucket(sess *session.Session) error {
	log.Println("Creating Bucket")
	svc := s3.New(sess)
	req := &s3.PutObjectInput{
		Bucket: aws.String("rsmachiner-user-code"),
		Key:    aws.String(user.CompanyID + "/"),
	}
	_, err := svc.PutObject(req)
	return err
}

//Entrypoint lambda to run code
func main() {
	switch os.Getenv("PLATFORM") {
	case "lambda":
		lambda.Start(HandleRequest)
	default:
		log.Println("no platform defined")
	}
}
